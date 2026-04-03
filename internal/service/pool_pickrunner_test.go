package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPickRunner_NoEnabledRunners(t *testing.T) {
	p := NewPool(NewRegistry(nil))
	_, _, err := p.pickRunner(context.Background(), "m")

	if err == nil || !strings.Contains(err.Error(), "нет включённых") {
		t.Fatalf("ожидалась ошибка про отсутствие раннеров, получено: %v", err)
	}
}

func TestRecordProbeFailure_BackoffIncreases(t *testing.T) {
	p := NewPool(NewRegistry(nil))
	addr := "127.0.0.1:59999"
	p.recordProbeFailure(addr)
	e1 := p.probeCache[addr]
	if e1.ok || !time.Now().Before(e1.backoffUntil) {
		t.Fatalf("ожидался backoff после сбоя: %+v", e1)
	}

	d1 := e1.backoffUntil.Sub(e1.probedAt)
	p.recordProbeFailure(addr)
	e2 := p.probeCache[addr]
	d2 := e2.backoffUntil.Sub(e2.probedAt)
	if d2 <= d1 {
		t.Fatalf("ожидали рост backoff: d1=%v d2=%v", d1, d2)
	}
}

func TestRecordProbeSuccess_ClearsFailure(t *testing.T) {
	p := NewPool(NewRegistry(nil))
	addr := "h:1"
	p.recordProbeFailure(addr)
	p.recordProbeSuccess(addr, []string{"a"}, 7)
	e := p.probeCache[addr]
	if !e.ok || e.failCount != 0 || len(e.models) != 1 || e.models[0] != "a" || e.maxGpuU != 7 {
		t.Fatalf("неожиданная запись кэша: %+v", e)
	}
}
