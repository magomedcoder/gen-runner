package usecase

import (
	"testing"
	"time"
)

func TestSessionPutRateLimiter_resetsAfterWindow(t *testing.T) {
	t0 := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	clock := t0
	lim := &sessionPutRateLimiter{
		now: func() time.Time {
			return clock
		},
	}

	const uid = 1
	const chunk = 1024 * 1024
	nOK := putSessionFileMaxBytesPerMin / chunk

	for i := 0; i < int(nOK); i++ {
		if err := lim.checkPutSessionFileRate(uid, chunk); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
	if err := lim.checkPutSessionFileRate(uid, chunk); err == nil {
		t.Fatal("expected error when byte budget exceeded within window")
	}

	clock = t0.Add(putSessionFileRateWindow)
	if err := lim.checkPutSessionFileRate(uid, chunk); err != nil {
		t.Fatalf("after window: %v", err)
	}
}

func TestSessionPutRateLimiter_uploadCountCap(t *testing.T) {
	t0 := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	clock := t0
	lim := &sessionPutRateLimiter{
		now: func() time.Time { return clock },
	}
	const uid = 42
	const small = 100

	for i := range putSessionFileMaxUploadsPerMin {
		if err := lim.checkPutSessionFileRate(uid, small); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}

	if err := lim.checkPutSessionFileRate(uid, small); err == nil {
		t.Fatal("expected error when upload count cap exceeded")
	}
}
