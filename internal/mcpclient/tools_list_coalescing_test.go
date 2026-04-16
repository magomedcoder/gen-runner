package mcpclient

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
)

func p95Duration(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}

	cp := append([]time.Duration(nil), ds...)
	for i := 0; i < len(cp); i++ {
		for j := i + 1; j < len(cp); j++ {
			if cp[j] < cp[i] {
				cp[i], cp[j] = cp[j], cp[i]
			}
		}
	}

	idx := int(float64(len(cp))*0.95) - 1
	if idx < 0 {
		idx = 0
	}

	if idx >= len(cp) {
		idx = len(cp) - 1
	}

	return cp[idx]
}

func runConcurrentListToolsWave(t *testing.T, c *ToolsListCache, srv *domain.MCPServer, workers int) (time.Duration, error) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Add(workers)

	durations := make([]time.Duration, workers)
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			start := time.Now()
			_, err := c.ListToolsCached(context.Background(), srv, time.Minute)
			durations[i] = time.Since(start)
			if err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return 0, err
		}
	}

	return p95Duration(durations), nil
}

func TestListToolsCoalescingReducesBurstP95(t *testing.T) {
	oldToolsFetcher := listToolsFetcher
	oldCoalescing := listRequestCoalescingEnabled.Load()
	t.Cleanup(func() {
		listToolsFetcher = oldToolsFetcher
		SetListRequestCoalescing(oldCoalescing)
	})

	var calls atomic.Int64
	var serialMu sync.Mutex
	listToolsFetcher = func(_ context.Context, _ *domain.MCPServer, _ *ToolsListCache) ([]DeclaredTool, error) {
		serialMu.Lock()
		defer serialMu.Unlock()
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		return []DeclaredTool{{Name: "echo"}}, nil
	}

	srv := &domain.MCPServer{
		ID:        101,
		Name:      "critical",
		Enabled:   true,
		Transport: "sse",
		URL:       "http://127.0.0.1:9000/mcp",
	}

	const workers = 24

	SetListRequestCoalescing(false)
	baselineCache := NewToolsListCache()
	baselineP95, err := runConcurrentListToolsWave(t, baselineCache, srv, workers)
	if err != nil {
		t.Fatal(err)
	}
	baselineCalls := calls.Load()

	calls.Store(0)
	SetListRequestCoalescing(true)
	optimizedCache := NewToolsListCache()
	optimizedP95, err := runConcurrentListToolsWave(t, optimizedCache, srv, workers)
	if err != nil {
		t.Fatal(err)
	}

	optimizedCalls := calls.Load()
	t.Logf("burst p95 baseline=%s optimized=%s backend_calls_baseline=%d backend_calls_optimized=%d", baselineP95, optimizedP95, baselineCalls, optimizedCalls)

	if optimizedP95 >= baselineP95 {
		t.Fatalf("expected lower p95 with coalescing, baseline=%s optimized=%s", baselineP95, optimizedP95)
	}

	if optimizedP95*10 > baselineP95*8 {
		t.Fatalf("p95 improvement is less than 20%%, baseline=%s optimized=%s", baselineP95, optimizedP95)
	}

	if optimizedCalls >= baselineCalls {
		t.Fatalf("expected fewer backend list calls, baseline=%d optimized=%d", baselineCalls, optimizedCalls)
	}
}
