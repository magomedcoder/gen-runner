package mcpclient

import (
	"context"
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
)

func TestUseHTTPSessionPool(t *testing.T) {
	ctx := context.Background()
	oldReuse := httpReuseSessions.Load()
	defer httpReuseSessions.Store(oldReuse)

	sse := &domain.MCPServer{
		ID:        42,
		Transport: "sse",
		URL:       "http://127.0.0.1:9/x",
	}
	stream := &domain.MCPServer{
		ID:        42,
		Transport: "streamable",
		URL:       "http://127.0.0.1:9/x",
	}
	stdio := &domain.MCPServer{
		ID:        42,
		Transport: "stdio",
		Command:   "/bin/true",
	}
	noID := &domain.MCPServer{
		ID:        0,
		Transport: "sse",
		URL:       "http://127.0.0.1:9/x",
	}

	httpReuseSessions.Store(false)
	if useHTTPSessionPool(ctx, sse) {
		t.Fatal("ожидалось false пока http_reuse_sessions выключен")
	}

	httpReuseSessions.Store(true)
	if !useHTTPSessionPool(ctx, sse) {
		t.Fatal("ожидалось true для sse с положительным id")
	}

	if !useHTTPSessionPool(ctx, stream) {
		t.Fatal("ожидалось true для streamable")
	}

	if useHTTPSessionPool(ctx, stdio) {
		t.Fatal("stdio не использует HTTP-пул")
	}

	if useHTTPSessionPool(ctx, noID) {
		t.Fatal("без id сессия не пулится")
	}
}

func TestUseHTTPSessionPoolWithSamplingContext(t *testing.T) {
	oldReuse := httpReuseSessions.Load()
	oldSampling := SamplingEnabled()
	defer func() {
		httpReuseSessions.Store(oldReuse)
		SetSamplingEnabled(oldSampling)
	}()

	httpReuseSessions.Store(true)
	SetSamplingEnabled(true)

	ctx := WithSamplingRunner(context.Background(), &SamplingRunner{
		SessionID:      7,
		RunnerAddr:     "127.0.0.1:50051",
		Model:          "test-model",
		StopSequences:  []string{"</s>"},
		TimeoutSeconds: 60,
		GenParams:      &domain.GenerationParams{},
	})

	sse := &domain.MCPServer{
		ID:        42,
		Transport: "sse",
		URL:       "http://127.0.0.1:9/x",
	}
	if !useHTTPSessionPool(ctx, sse) {
		t.Fatal("ожидалось true: HTTP-пул должен работать и при sampling")
	}
}

func TestPoolSessionFingerprintIncludesSamplingContext(t *testing.T) {
	oldSampling := SamplingEnabled()
	defer SetSamplingEnabled(oldSampling)
	SetSamplingEnabled(true)

	srv := &domain.MCPServer{
		ID:        42,
		Transport: "streamable",
		URL:       "http://127.0.0.1:9/x",
	}

	ctxA := WithSamplingRunner(context.Background(), &SamplingRunner{
		SessionID:      1,
		RunnerAddr:     "runner-a",
		Model:          "model-a",
		TimeoutSeconds: 60,
		GenParams:      &domain.GenerationParams{},
	})
	ctxB := WithSamplingRunner(context.Background(), &SamplingRunner{
		SessionID:      1,
		RunnerAddr:     "runner-b",
		Model:          "model-a",
		TimeoutSeconds: 60,
		GenParams:      &domain.GenerationParams{},
	})

	fpA := poolSessionFingerprint(ctxA, srv)
	fpB := poolSessionFingerprint(ctxB, srv)
	if fpA == fpB {
		t.Fatal("fingerprint должен меняться при смене sampling context")
	}
}
