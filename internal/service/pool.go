package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/magomedcoder/gen/api/pb/llmrunnerpb"
	"github.com/magomedcoder/gen/api/pb/runnerpb"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	runnerProbeTTL          = 5 * time.Second
	runnerProbeBackoffBase  = time.Second
	runnerProbeBackoffMax   = 30 * time.Second
	runnerProbeBackoffShift = 5
)

type runnerProbeEntry struct {
	ok           bool
	models       []string
	maxGpuU      uint32
	probedAt     time.Time
	backoffUntil time.Time
	failCount    int
}

type Pool struct {
	reg        *Registry
	mu         sync.Mutex
	clients    map[string]*LLMRunnerService
	inflight   sync.Map
	probeMu    sync.Mutex
	probeCache map[string]runnerProbeEntry
}

func NewPool(reg *Registry) *Pool {
	return &Pool{
		reg:        reg,
		clients:    make(map[string]*LLMRunnerService),
		probeCache: make(map[string]runnerProbeEntry),
	}
}

func (p *Pool) getOrCreateInflight(addr string) *atomic.Int32 {
	v, _ := p.inflight.LoadOrStore(addr, new(atomic.Int32))
	return v.(*atomic.Int32)
}

func (p *Pool) getClient(addr string) (*LLMRunnerService, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[addr]; ok {
		return c, nil
	}

	c, err := NewLLMRunnerService(addr, "")
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

	p.probeMu.Lock()
	clear(p.probeCache)
	p.probeMu.Unlock()

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
	p.probeMu.Lock()
	delete(p.probeCache, addr)

	p.probeMu.Unlock()
}

func (p *Pool) recordProbeFailure(addr string) {
	p.probeMu.Lock()
	defer p.probeMu.Unlock()

	prev := p.probeCache[addr]
	fc := prev.failCount + 1
	shift := max(fc-1, 0)

	if shift > runnerProbeBackoffShift {
		shift = runnerProbeBackoffShift
	}

	backoff := min(runnerProbeBackoffBase*time.Duration(uint(1)<<uint(shift)), runnerProbeBackoffMax)

	now := time.Now()
	p.probeCache[addr] = runnerProbeEntry{
		ok:           false,
		probedAt:     now,
		backoffUntil: now.Add(backoff),
		failCount:    fc,
	}
}

func (p *Pool) recordProbeSuccess(addr string, models []string, maxGpuU uint32) {
	p.probeMu.Lock()
	defer p.probeMu.Unlock()

	p.probeCache[addr] = runnerProbeEntry{
		ok:        true,
		models:    append([]string(nil), models...),
		maxGpuU:   maxGpuU,
		probedAt:  time.Now(),
		failCount: 0,
	}
}

func (p *Pool) candidateFromRunnerProbe(addr string, c *LLMRunnerService, pr *llmrunnerpb.RunnerProbeResponse, model string, modelMatters bool) *candidate {
	if pr == nil || !pr.GetBackendConnected() {
		p.recordProbeFailure(addr)
		return nil
	}

	var models []string
	if si := pr.GetServer(); si != nil {
		models = si.GetModels()
	}
	gpuU := maxGPUUtil(pr.GetGpus())

	if modelMatters && !modelAllowed(model, models) {
		p.recordProbeSuccess(addr, models, gpuU)
		return nil
	}

	p.recordProbeSuccess(addr, models, gpuU)
	inf := p.getOrCreateInflight(addr).Load()
	score := float64(inf)*100 + float64(gpuU)
	return &candidate{addr: addr, client: c, score: score}
}

func (p *Pool) probeRunnerAddressLegacy(ctx context.Context, c *LLMRunnerService, addr, model string, modelMatters bool) *candidate {
	ok, err := c.CheckConnection(ctx)
	if err != nil || !ok {
		p.recordProbeFailure(addr)
		return nil
	}

	var si *llmrunnerpb.ServerInfo
	var gi *llmrunnerpb.GetGpuInfoResponse
	var parWG sync.WaitGroup
	parWG.Add(2)
	go func() {
		defer parWG.Done()
		si, _ = c.GetServerInfo(ctx)
	}()

	go func() {
		defer parWG.Done()
		gi, _ = c.GetGpuInfo(ctx)
	}()

	parWG.Wait()

	var models []string
	if si != nil {
		models = si.GetModels()
	}
	gpuU := maxGPUUtil(gi.GetGpus())

	if modelMatters && !modelAllowed(model, models) {
		p.recordProbeSuccess(addr, models, gpuU)
		return nil
	}

	p.recordProbeSuccess(addr, models, gpuU)
	inf := p.getOrCreateInflight(addr).Load()
	score := float64(inf)*100 + float64(gpuU)

	return &candidate{
		addr:   addr,
		client: c,
		score:  score,
	}
}

func (p *Pool) probeRunnerAddress(ctx context.Context, addr, model string, modelMatters bool) *candidate {
	c, err := p.getClient(addr)
	if err != nil {
		return nil
	}

	pr, err := c.RunnerProbe(ctx)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.Unimplemented {
			return p.probeRunnerAddressLegacy(ctx, c, addr, model, modelMatters)
		}

		p.recordProbeFailure(addr)
		return nil
	}

	return p.candidateFromRunnerProbe(addr, c, pr, model, modelMatters)
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

func llmLoadedModelToRunnerPB(in *llmrunnerpb.GetLoadedModelResponse) *runnerpb.LoadedModelStatus {
	if in == nil {
		return nil
	}

	return &runnerpb.LoadedModelStatus{
		Loaded:       in.GetLoaded(),
		DisplayName:  in.GetDisplayName(),
		GgufBasename: in.GetGgufBasename(),
	}
}

func (p *Pool) ProbeLLMRunner(ctx context.Context, address string) (connected bool, gpus []*runnerpb.GpuInfo, server *runnerpb.ServerInfo, loaded *runnerpb.LoadedModelStatus) {
	c, err := p.getClient(address)
	if err != nil {
		return false, nil, nil, nil
	}

	pr, err := c.RunnerProbe(ctx)
	if err == nil {
		if pr == nil || !pr.GetBackendConnected() {
			return false, nil, nil, nil
		}

		gpus = llmGpusToRunnerPB(pr.GetGpus())
		server = llmServerToRunnerPB(pr.GetServer())
		loaded = llmLoadedModelToRunnerPB(pr.GetLoadedModel())
		return true, gpus, server, loaded
	}

	if st, ok := status.FromError(err); !ok || st.Code() != codes.Unimplemented {
		return false, nil, nil, nil
	}

	ok, err := c.CheckConnection(ctx)
	if err != nil || !ok {
		return false, nil, nil, nil
	}

	gi, err := c.GetGpuInfo(ctx)
	if err == nil && gi != nil {
		gpus = llmGpusToRunnerPB(gi.GetGpus())
	}

	si, err := c.GetServerInfo(ctx)
	if err == nil {
		server = llmServerToRunnerPB(si)
	}

	lm, err := c.GetLoadedModel(ctx)
	if err == nil {
		loaded = llmLoadedModelToRunnerPB(lm)
	}

	return true, gpus, server, loaded
}

func (p *Pool) WaitRunnerIdle(ctx context.Context, address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return fmt.Errorf("пустой адрес раннера")
	}

	ai := p.getOrCreateInflight(address)
	if ai.Load() == 0 {
		return nil
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if ai.Load() == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *Pool) UnloadModelOnRunner(ctx context.Context, address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return fmt.Errorf("пустой адрес раннера")
	}

	c, err := p.getClient(address)
	if err != nil {
		return err
	}

	return c.UnloadModel(ctx)
}

func (p *Pool) WarmModelOnRunner(ctx context.Context, address string, model string) error {
	address = strings.TrimSpace(address)
	model = strings.TrimSpace(model)
	if address == "" {
		return fmt.Errorf("пустой адрес раннера")
	}

	if model == "" || model == "default" {
		return nil
	}

	c, err := p.getClient(address)
	if err != nil {
		return err
	}

	_, err = c.Embed(ctx, model, "warmup")

	return err
}

type candidate struct {
	addr   string
	client *LLMRunnerService
	score  float64
}

func (p *Pool) pickRunner(ctx context.Context, model string) (*LLMRunnerService, string, error) {
	addrs := p.reg.GetEnabledAddresses()
	if len(addrs) == 0 {
		return nil, "", fmt.Errorf("нет включённых llm-runner в реестре")
	}

	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	now := time.Now()
	var fromCache []candidate
	var toProbe []string

	for _, addr := range addrs {
		p.probeMu.Lock()
		e := p.probeCache[addr]
		p.probeMu.Unlock()

		if e.ok && now.Sub(e.probedAt) < runnerProbeTTL {
			if !modelAllowed(model, e.models) {
				continue
			}

			c, err := p.getClient(addr)
			if err != nil {
				continue
			}

			inf := p.getOrCreateInflight(addr).Load()
			fromCache = append(fromCache, candidate{
				addr:   addr,
				client: c,
				score:  float64(inf)*100 + float64(e.maxGpuU),
			})

			continue
		}

		if !e.ok && now.Before(e.backoffUntil) {
			continue
		}

		toProbe = append(toProbe, addr)
	}

	var probed []candidate
	if len(toProbe) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, addr := range toProbe {
			wg.Add(1)
			go func(addr string) {
				defer wg.Done()
				if c := p.probeRunnerAddress(probeCtx, addr, model, true); c != nil {
					mu.Lock()
					probed = append(probed, *c)
					mu.Unlock()
				}
			}(addr)
		}

		wg.Wait()
	}

	found := append(fromCache, probed...)

	if len(found) == 0 {
		var fbMu sync.Mutex
		var fallback []candidate
		var wg sync.WaitGroup
		for _, addr := range addrs {
			wg.Add(1)
			go func(addr string) {
				defer wg.Done()
				if c := p.probeRunnerAddress(probeCtx, addr, model, false); c != nil {
					fbMu.Lock()
					fallback = append(fallback, *c)
					fbMu.Unlock()
				}
			}(addr)
		}
		wg.Wait()
		found = fallback
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

func (p *Pool) SendMessageWithRunnerToolAction(
	ctx context.Context,
	sessionID int64,
	model string,
	messages []*domain.Message,
	stopSequences []string,
	timeoutSeconds int32,
	genParams *domain.GenerationParams,
) (chan string, func() string, error) {
	client, addr, err := p.pickRunner(ctx, model)
	if err != nil {
		return nil, nil, err
	}

	ai := p.getOrCreateInflight(addr)
	ai.Add(1)
	ch, toolFn, err := client.SendMessageWithRunnerToolAction(ctx, sessionID, model, messages, stopSequences, timeoutSeconds, genParams)
	if err != nil {
		ai.Add(-1)
		return nil, nil, err
	}

	return forwardStream(ch, ai), toolFn, nil
}

func (p *Pool) SendMessageOnRunner(
	ctx context.Context,
	runnerAddr string,
	sessionID int64,
	model string,
	messages []*domain.Message,
	stopSequences []string,
	timeoutSeconds int32,
	genParams *domain.GenerationParams,
) (chan string, error) {
	runnerAddr = strings.TrimSpace(runnerAddr)
	if runnerAddr == "" {
		return nil, fmt.Errorf("runner address is empty")
	}

	client, err := p.getClient(runnerAddr)
	if err != nil {
		return nil, err
	}

	ai := p.getOrCreateInflight(runnerAddr)
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
