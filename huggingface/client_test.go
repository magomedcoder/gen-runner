package huggingface

import (
	"context"
	"errors"
	"io"
	"syscall"
	"testing"
)

func TestModelInfoCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := NewClient("")
	_, err := c.ModelInfo(ctx, "some/repo")
	if err == nil {
		t.Fatal("ожидалась ошибка при отменённом контексте")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ожидался context.Canceled, получено: %v", err)
	}
}

func TestParseContentRange(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in        string
		wantStart int64
		wantEnd   int64
		wantTotal int64
		wantOK    bool
	}{
		{"bytes 0-9/100", 0, 9, 100, true},
		{"BYTES 1000-1999/30300000000", 1000, 1999, 30300000000, true},
		{"bytes */100", 0, 0, 100, true},
		{"bytes 0-9/*", 0, 9, -1, true},
		{"", 0, 0, 0, false},
		{"text 0-1/2", 0, 0, 0, false},
	}

	for _, tc := range cases {
		st, en, tot, ok := parseContentRange(tc.in)
		if ok != tc.wantOK || st != tc.wantStart || en != tc.wantEnd || tot != tc.wantTotal {
			t.Fatalf("parseContentRange(%q) = (%d,%d,%d,%v), want (%d,%d,%d,%v)",
				tc.in, st, en, tot, ok, tc.wantStart, tc.wantEnd, tc.wantTotal, tc.wantOK)
		}
	}
}

func TestRetryableDownloadErr(t *testing.T) {
	t.Parallel()

	if !retryableDownloadErr(io.ErrUnexpectedEOF) {
		t.Fatal("unexpected EOF должна считаться повторяемой")
	}

	if !retryableDownloadErr(syscall.ECONNRESET) {
		t.Fatal("ECONNRESET должна считаться повторяемой")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if retryableDownloadErr(ctx.Err()) {
		t.Fatal("отмена контекста не должна считаться повторяемой")
	}
}
