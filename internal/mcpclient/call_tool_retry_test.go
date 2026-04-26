package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type callAttemptCtxKey struct{}

func TestCallToolTransportRetryReducesErrorRateByAtLeast30Percent(t *testing.T) {
	oldRunner := callToolSessionRunner
	oldInvoker := callToolInvoker
	oldRetry := callToolTransportRetryEnabled.Load()
	t.Cleanup(func() {
		callToolSessionRunner = oldRunner
		callToolInvoker = oldInvoker
		SetCallToolTransportRetry(oldRetry)
	})

	callToolSessionRunner = func(ctx context.Context, _ *domain.MCPServer, _ *ToolsListCache, fn func(context.Context, *mcp.ClientSession) error) error {
		flag, _ := ctx.Value(callAttemptCtxKey{}).(*atomic.Bool)
		if flag == nil {
			return errors.New("missing attempt flag")
		}

		if !flag.Swap(true) {
			return context.DeadlineExceeded
		}
		return fn(ctx, nil)
	}
	callToolInvoker = func(_ context.Context, _ *mcp.ClientSession, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: "ok",
			}},
		}, nil
	}

	srv := &domain.MCPServer{ID: 7}
	tool := ToolAlias(srv.ID, "list_resources")
	const total = 20
	args := json.RawMessage(`{}`)

	runWave := func(retryEnabled bool) float64 {
		SetCallToolTransportRetry(retryEnabled)
		failures := 0
		for range total {
			ctx := context.WithValue(context.Background(), callAttemptCtxKey{}, &atomic.Bool{})
			if _, err := CallTool(ctx, srv, tool, args); err != nil {
				failures++
			}
		}

		return float64(failures) / float64(total)
	}

	baselineFailRate := runWave(false)
	optimizedFailRate := runWave(true)
	t.Logf("timeout/transport fail rate baseline=%.2f optimized=%.2f", baselineFailRate, optimizedFailRate)

	if baselineFailRate <= 0 {
		t.Fatalf("expected non-zero baseline fail rate")
	}

	if optimizedFailRate >= baselineFailRate {
		t.Fatalf("expected lower fail rate with retry, baseline=%.2f optimized=%.2f", baselineFailRate, optimizedFailRate)
	}

	if optimizedFailRate > baselineFailRate*0.70 {
		t.Fatalf("fail-rate reduction is below 30%%, baseline=%.2f optimized=%.2f", baselineFailRate, optimizedFailRate)
	}
}
