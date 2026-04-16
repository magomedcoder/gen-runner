package mcpclient

import (
	"context"
	"errors"
	"testing"
)

type timeoutErr struct{}

func (timeoutErr) Error() string {
	return "timeout"
}

func (timeoutErr) Timeout() bool {
	return true
}

func (timeoutErr) Temporary() bool {
	return true
}

func TestShouldRetryPooledSessionError(t *testing.T) {
	if shouldRetryPooledSessionError(context.Background(), nil) {
		t.Fatal("nil error must not be retried")
	}

	if shouldRetryPooledSessionError(context.Background(), context.Canceled) {
		t.Fatal("context canceled must not be retried")
	}

	if !shouldRetryPooledSessionError(context.Background(), context.DeadlineExceeded) {
		t.Fatal("deadline exceeded should be retried once for pooled sessions")
	}

	if !shouldRetryPooledSessionError(context.Background(), timeoutErr{}) {
		t.Fatal("net timeout error should be retried")
	}

	if !shouldRetryPooledSessionError(context.Background(), errors.New("read: connection reset by peer")) {
		t.Fatal("connection reset should be retried")
	}

	if shouldRetryPooledSessionError(context.Background(), errors.New("validation failed")) {
		t.Fatal("business errors must not be retried")
	}
}

func TestShouldRetryPooledSessionErrorParentContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if shouldRetryPooledSessionError(ctx, errors.New("connection reset by peer")) {
		t.Fatal("must not retry when parent context is already done")
	}
}
