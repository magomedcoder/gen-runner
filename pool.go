package runner

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/magomedcoder/llm-runner/domain"
	"github.com/magomedcoder/llm-runner/logger"
	"github.com/magomedcoder/llm-runner/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	maxRetriesPerRunner   = 2
	retryBackoff          = 100 * time.Millisecond
	getRunnersParallelism = 8
)

type Pool struct {
	addresses []string
	disabled  map[string]bool
	mu        sync.RWMutex
	index     atomic.Uint32
	conns     map[string]*grpc.ClientConn
	connMu    sync.Mutex
}

func NewPool(addresses []string) *Pool {
	p := &Pool{
		addresses: make([]string, 0, len(addresses)),
		disabled:  make(map[string]bool),
		conns:     make(map[string]*grpc.ClientConn),
	}

	for _, a := range addresses {
		if a != "" {
			p.addresses = append(p.addresses, a)
		}
	}

	return p
}

func (p *Pool) Add(address string) {
	if address == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if slices.Contains(p.addresses, address) {
		return
	}

	p.addresses = append(p.addresses, address)
}

func (p *Pool) Remove(address string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, a := range p.addresses {
		if a == address {
			p.addresses = append(p.addresses[:i], p.addresses[i+1:]...)
			break
		}
	}

	p.closeConn(address)
}

func (p *Pool) closeConn(address string) {
	p.connMu.Lock()
	defer p.connMu.Unlock()

	if conn, ok := p.conns[address]; ok {
		_ = conn.Close()
		delete(p.conns, address)
	}
}

func (p *Pool) getConn(ctx context.Context, address string) (pb.LLMRunnerServiceClient, error) {
	p.connMu.Lock()
	if conn, ok := p.conns[address]; ok {
		p.connMu.Unlock()
		return pb.NewLLMRunnerServiceClient(conn), nil
	}
	p.connMu.Unlock()

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.W("Pool: ошибка подключения к раннеру %s: %v", address, err)
		return nil, fmt.Errorf("подключение к раннеру %s: %w", address, err)
	}

	p.connMu.Lock()
	defer p.connMu.Unlock()
	if existing, ok := p.conns[address]; ok {
		_ = conn.Close()
		return pb.NewLLMRunnerServiceClient(existing), nil
	}
	logger.D("Pool: подключение к раннеру %s установлено", address)
	p.conns[address] = conn
	return pb.NewLLMRunnerServiceClient(conn), nil
}

func (p *Pool) enabledAddresses() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make([]string, 0, len(p.addresses))
	for _, a := range p.addresses {
		if !p.disabled[a] {
			out = append(out, a)
		}
	}

	return out
}

func (p *Pool) pickRunner() (string, bool) {
	addrs := p.enabledAddresses()
	if len(addrs) == 0 {
		return "", false
	}

	i := p.index.Add(1) % uint32(len(addrs))
	return addrs[i], true
}

func (p *Pool) GetRunners(ctx context.Context) []RunnerInfo {
	p.mu.RLock()
	addrs := make([]string, len(p.addresses))
	copy(addrs, p.addresses)
	disabledCopy := make(map[string]bool)
	maps.Copy(disabledCopy, p.disabled)
	p.mu.RUnlock()

	probeTimeout := 3 * time.Second
	sem := make(chan struct{}, getRunnersParallelism)
	var wg sync.WaitGroup
	for _, addr := range addrs {
		if disabledCopy[addr] {
			continue
		}
		wg.Add(1)
		go func(a string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			client, err := p.getConn(probeCtx, a)
			cancel()
			if err != nil {
				return
			}

			pingCtx, cancelPing := context.WithTimeout(ctx, probeTimeout)
			resp, err := client.CheckConnection(pingCtx, &pb.Empty{})
			cancelPing()
			if err != nil || resp == nil || !resp.IsConnected {
				p.closeConn(a)
			}
		}(addr)
	}
	wg.Wait()

	p.connMu.Lock()
	connStatus := make(map[string]bool)
	for a := range p.conns {
		connStatus[a] = true
	}
	p.connMu.Unlock()

	out := make([]RunnerInfo, len(addrs))
	for i, a := range addrs {
		enabled := !disabledCopy[a]
		out[i] = RunnerInfo{
			Address:   a,
			Enabled:   enabled,
			Connected: connStatus[a] && enabled,
		}
	}

	return out
}

func (p *Pool) SetRunnerEnabled(address string, enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if slices.Contains(p.addresses, address) {
		if enabled {
			delete(p.disabled, address)
		} else {
			p.disabled[address] = true
			p.closeConn(address)
		}
		return
	}
}

func (p *Pool) HasActiveRunners() bool {
	return len(p.enabledAddresses()) > 0
}

type RunnerInfo struct {
	Address   string
	Enabled   bool
	Connected bool
}

func (p *Pool) CheckConnection(ctx context.Context) (bool, error) {
	addrs := p.enabledAddresses()
	if len(addrs) == 0 {
		return false, fmt.Errorf("нет активных раннеров")
	}

	for _, addr := range addrs {
		client, err := p.getConn(ctx, addr)
		if err != nil {
			continue
		}

		resp, err := client.CheckConnection(ctx, &pb.Empty{})
		if err == nil && resp != nil && resp.IsConnected {
			return true, nil
		}
	}

	return false, fmt.Errorf("ни один раннер не отвечает")
}

func (p *Pool) GetModels(ctx context.Context) ([]string, error) {
	addrs := p.enabledAddresses()
	if len(addrs) == 0 {
		return nil, fmt.Errorf("нет активных раннеров")
	}

	for _, addr := range addrs {
		client, err := p.getConn(ctx, addr)
		if err != nil {
			continue
		}

		resp, err := client.GetModels(ctx, &pb.Empty{})
		if err == nil && resp != nil {
			return resp.Models, nil
		}
	}

	return nil, fmt.Errorf("ни один раннер не вернул список моделей")
}

func (p *Pool) GetGpuInfo(ctx context.Context, address string) *pb.GetGpuInfoResponse {
	client, err := p.getConn(ctx, address)
	if err != nil {
		return nil
	}

	resp, err := client.GetGpuInfo(ctx, &pb.Empty{})
	if err != nil || resp == nil {
		return nil
	}

	return resp
}

func (p *Pool) GetServerInfo(ctx context.Context, address string) *pb.ServerInfo {
	client, err := p.getConn(ctx, address)
	if err != nil {
		return nil
	}

	resp, err := client.GetServerInfo(ctx, &pb.Empty{})
	if err != nil || resp == nil {
		return nil
	}

	return resp
}

func (p *Pool) trySendMessage(ctx context.Context, addr string, sessionID int64, model string, messages []*domain.AIChatMessage, stopSequences []string, timeoutSeconds int32, genParams *domain.GenerationParams) (chan string, error) {
	client, err := p.getConn(ctx, addr)
	if err != nil {
		return nil, err
	}

	protoMessages := make([]*pb.ChatMessage, len(messages))
	for i, m := range messages {
		protoMessages[i] = domain.AIMessageToProto(m)
	}
	req := &pb.SendMessageRequest{
		SessionId:     sessionID,
		Messages:      protoMessages,
		Model:         model,
		StopSequences: stopSequences,
	}
	if timeoutSeconds > 0 {
		req.TimeoutSeconds = &timeoutSeconds
	}
	_ = genParams

	stream, err := client.SendMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm-runner %s: %w", addr, err)
	}

	out := make(chan string, 100)
	go func() {
		defer close(out)
		for {
			msg, err := stream.Recv()
			if err != nil {
				return
			}

			if msg.Content != "" {
				select {
				case <-ctx.Done():
					return
				case out <- msg.Content:
				}
			}
			if msg.Done {
				return
			}
		}
	}()

	return out, nil
}

func (p *Pool) orderedAddresses(startAddr string) []string {
	addrs := p.enabledAddresses()
	if len(addrs) == 0 {
		return nil
	}
	if startAddr == "" {
		return addrs
	}

	order := make([]string, 0, len(addrs))
	seen := make(map[string]bool)
	order = append(order, startAddr)
	seen[startAddr] = true
	for _, a := range addrs {
		if !seen[a] {
			order = append(order, a)
		}
	}

	return order
}

func (p *Pool) SendMessage(ctx context.Context, sessionID int64, model string, messages []*domain.AIChatMessage, stopSequences []string, timeoutSeconds int32, genParams *domain.GenerationParams) (chan string, error) {
	firstAddr, ok := p.pickRunner()
	if !ok {
		logger.W("Pool: нет доступных раннеров для сессии %d", sessionID)
		return nil, fmt.Errorf("нет доступных раннеров")
	}

	order := p.orderedAddresses(firstAddr)
	var lastErr error

	for _, addr := range order {
		for attempt := 0; attempt <= maxRetriesPerRunner; attempt++ {
			if attempt > 0 {
				time.Sleep(retryBackoff)
			}

			ch, err := p.trySendMessage(ctx, addr, sessionID, model, messages, stopSequences, timeoutSeconds, genParams)
			if err == nil {
				logger.V("Pool: запрос для сессии %d обработан раннером %s (попытка %d)", sessionID, addr, attempt+1)
				return ch, nil
			}

			lastErr = err
			logger.W("Pool: раннер %s попытка %d/%d: %v", addr, attempt+1, maxRetriesPerRunner+1, err)
		}

		p.closeConn(addr)
	}

	return nil, fmt.Errorf("все раннеры недоступны после retry и failover: %w", lastErr)
}
