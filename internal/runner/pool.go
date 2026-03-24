package runner

import (
	"context"
	"fmt"
	"github.com/magomedcoder/gen/api/pb/llmrunnerpb"
	"github.com/magomedcoder/gen/api/pb/runnerpb"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/pkg/logger"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

type Pool struct {
	reg      *Registry
	mu       sync.Mutex
	clients  map[string]*service.LLMRunnerService
	inflight sync.Map
}

func NewPool(reg *Registry) *Pool {
	return &Pool{
		reg:     reg,
		clients: make(map[string]*service.LLMRunnerService),
	}
}

func (p *Pool) getOrCreateInflight(addr string) *atomic.Int32 {
	v, _ := p.inflight.LoadOrStore(addr, new(atomic.Int32))
	return v.(*atomic.Int32)
}

func (p *Pool) getClient(addr string) (*service.LLMRunnerService, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[addr]; ok {
		return c, nil
	}

	c, err := service.NewLLMRunnerService(addr, "")
	if err != nil {
		return nil, err
	}
	p.clients[addr] = c
	p.getOrCreateInflight(addr)
	return c, nil
}

func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var firstErr error
	for addr, c := range p.clients {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(p.clients, addr)
		p.inflight.Delete(addr)
	}
	return firstErr
}

func (p *Pool) CloseAddr(addr string) {
	p.closeAddr(addr)
}

func (p *Pool) CloseAddrForget(addr string) {
	p.closeAddr(addr)
}

func (p *Pool) closeAddr(addr string) {
	p.mu.Lock()
	if c, ok := p.clients[addr]; ok {
		_ = c.Close()
		delete(p.clients, addr)
	}
	p.mu.Unlock()
	p.inflight.Delete(addr)
}

func modelAllowed(requested string, serverModels []string) bool {
	if requested == "" || requested == "default" {
		return true
	}
	if len(serverModels) == 0 {
		return true
	}
	return slices.Contains(serverModels, requested)
}

func maxGPUUtil(gpus []*llmrunnerpb.GpuInfo) uint32 {
	var max uint32
	for _, g := range gpus {
		if g == nil {
			continue
		}
		u := g.GetUtilizationPercent()
		if u > max {
			max = u
		}
	}
	return max
}

func llmGpusToRunnerPB(in []*llmrunnerpb.GpuInfo) []*runnerpb.GpuInfo {
	out := make([]*runnerpb.GpuInfo, 0, len(in))
	for _, g := range in {
		if g == nil {
			continue
		}
		out = append(out, &runnerpb.GpuInfo{
			Name:               g.GetName(),
			TemperatureC:       g.GetTemperatureC(),
			MemoryTotalMb:      g.GetMemoryTotalMb(),
			MemoryUsedMb:       g.GetMemoryUsedMb(),
			UtilizationPercent: g.GetUtilizationPercent(),
		})
	}
	return out
}

func llmServerToRunnerPB(si *llmrunnerpb.ServerInfo) *runnerpb.ServerInfo {
	if si == nil {
		return nil
	}
	return &runnerpb.ServerInfo{
		Hostname:      si.GetHostname(),
		Os:            si.GetOs(),
		Arch:          si.GetArch(),
		CpuCores:      si.GetCpuCores(),
		MemoryTotalMb: si.GetMemoryTotalMb(),
		Models:        append([]string(nil), si.GetModels()...),
	}
}

func (p *Pool) ProbeLLMRunner(ctx context.Context, address string) (connected bool, gpus []*runnerpb.GpuInfo, server *runnerpb.ServerInfo) {
	c, err := p.getClient(address)
	if err != nil {
		return false, nil, nil
	}
	ok, err := c.CheckConnection(ctx)
	if err != nil || !ok {
		return false, nil, nil
	}
	gi, err := c.GetGpuInfo(ctx)
	if err == nil && gi != nil {
		gpus = llmGpusToRunnerPB(gi.GetGpus())
	}
	si, err := c.GetServerInfo(ctx)
	if err == nil {
		server = llmServerToRunnerPB(si)
	}
	return true, gpus, server
}

type candidate struct {
	addr   string
	client *service.LLMRunnerService
	score  float64
}

func (p *Pool) pickRunner(ctx context.Context, model string) (*service.LLMRunnerService, string, error) {
	addrs := p.reg.GetEnabledAddresses()
	if len(addrs) == 0 {
		return nil, "", fmt.Errorf("нет включённых llm-runner в реестре")
	}

	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var found []candidate

	for _, addr := range addrs {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			c, err := p.getClient(addr)
			if err != nil {
				return
			}
			ok, err := c.CheckConnection(probeCtx)
			if err != nil || !ok {
				return
			}
			si, _ := c.GetServerInfo(probeCtx)
			var models []string
			if si != nil {
				models = si.GetModels()
			}
			if !modelAllowed(model, models) {
				return
			}
			gi, _ := c.GetGpuInfo(probeCtx)
			gpuU := maxGPUUtil(gi.GetGpus())
			inf := p.getOrCreateInflight(addr).Load()

			score := float64(inf)*100 + float64(gpuU)

			mu.Lock()
			found = append(found, candidate{addr: addr, client: c, score: score})
			mu.Unlock()
		}(addr)
	}
	wg.Wait()

	if len(found) == 0 {
		for _, addr := range addrs {
			c, err := p.getClient(addr)
			if err != nil {
				continue
			}
			ok, err := c.CheckConnection(probeCtx)
			if err != nil || !ok {
				continue
			}
			gi, _ := c.GetGpuInfo(probeCtx)
			gpuU := maxGPUUtil(gi.GetGpus())
			inf := p.getOrCreateInflight(addr).Load()
			found = append(found, candidate{addr: addr, client: c, score: float64(inf)*100 + float64(gpuU)})
		}
	}

	if len(found) == 0 {
		return nil, "", fmt.Errorf("ни один llm-runner не отвечает по gRPC")
	}

	best := found[0]
	for _, c := range found[1:] {
		if c.score < best.score {
			best = c
		}
	}
	logger.V("Pool: выбран раннер %s score=%.1f model=%q", best.addr, best.score, model)
	return best.client, best.addr, nil
}

func forwardStream(ch <-chan string, ai *atomic.Int32) chan string {
	out := make(chan string, 100)
	go func() {
		defer close(out)
		if ai != nil {
			defer ai.Add(-1)
		}
		for chunk := range ch {
			select {
			case out <- chunk:
			}
		}
	}()
	return out
}

func (p *Pool) CheckConnection(ctx context.Context) (bool, error) {
	addrs := p.reg.GetEnabledAddresses()
	if len(addrs) == 0 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, addr := range addrs {
		c, err := p.getClient(addr)
		if err != nil {
			continue
		}
		ok, err := c.CheckConnection(ctx)
		if err == nil && ok {
			return true, nil
		}
	}
	return false, nil
}

func (p *Pool) GetModels(ctx context.Context) ([]string, error) {
	addrs := p.reg.GetEnabledAddresses()
	if len(addrs) == 0 {
		return nil, fmt.Errorf("нет включённых llm-runner")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	seen := make(map[string]struct{})
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, addr := range addrs {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			c, err := p.getClient(addr)
			if err != nil {
				return
			}
			ok, err := c.CheckConnection(ctx)
			if err != nil || !ok {
				return
			}
			models, err := c.GetModels(ctx)
			if err != nil {
				logger.W("Pool GetModels: %s: %v", addr, err)
				return
			}
			mu.Lock()
			for _, m := range models {
				if m == "" {
					continue
				}
				seen[m] = struct{}{}
			}
			mu.Unlock()
		}(addr)
	}
	wg.Wait()

	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	slices.Sort(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("не удалось получить модели ни с одного llm-runner")
	}
	return out, nil
}

func (p *Pool) SendMessage(
	ctx context.Context,
	sessionID int64,
	model string,
	messages []*domain.Message,
	stopSequences []string,
	timeoutSeconds int32,
	genParams *domain.GenerationParams,
) (chan string, error) {
	client, addr, err := p.pickRunner(ctx, model)
	if err != nil {
		return nil, err
	}
	ai := p.getOrCreateInflight(addr)
	ai.Add(1)
	ch, err := client.SendMessage(ctx, sessionID, model, messages, stopSequences, timeoutSeconds, genParams)
	if err != nil {
		ai.Add(-1)
		return nil, err
	}

	return forwardStream(ch, ai), nil
}

func (p *Pool) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	client, addr, err := p.pickRunner(ctx, model)
	if err != nil {
		return nil, err
	}

	ai := p.getOrCreateInflight(addr)
	ai.Add(1)
	defer ai.Add(-1)

	return client.Embed(ctx, model, text)
}

func (p *Pool) EmbedBatch(ctx context.Context, model string, texts []string) ([][]float32, error) {
	client, addr, err := p.pickRunner(ctx, model)
	if err != nil {
		return nil, err
	}

	ai := p.getOrCreateInflight(addr)
	ai.Add(1)
	defer ai.Add(-1)

	return client.EmbedBatch(ctx, model, texts)
}
