package mcpclient

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
)

var callToolTransportRetryEnabled atomic.Bool

func init() {
	callToolTransportRetryEnabled.Store(true)
}

func SetCallToolTransportRetry(enabled bool) {
	callToolTransportRetryEnabled.Store(enabled)
}

func isRetryableTransportError(parentCtx context.Context, err error) bool {
	if err == nil {
		return false
	}

	if parentCtx != nil && parentCtx.Err() != nil {
		return false
	}

	if errors.Is(err, context.Canceled) {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}

	hints := []string{
		"timeout",
		"deadline exceeded",
		"i/o timeout",
		"broken pipe",
		"connection reset",
		"connection aborted",
		"unexpected eof",
		"stream closed",
		"transport is closing",
		"http2: client connection lost",
	}
	for _, h := range hints {
		if strings.Contains(msg, h) {
			return true
		}
	}

	return false
}

func callToolAllowsTransportRetry(toolName string) bool {
	if !callToolTransportRetryEnabled.Load() {
		return false
	}

	name := strings.ToLower(strings.TrimSpace(toolName))
	if name == "" {
		return false
	}

	if _, _, ok := ParseToolAlias(name); ok {
		return true
	}

	return false
}
