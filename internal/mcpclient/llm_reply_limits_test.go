package mcpclient

import (
	"strings"
	"testing"
)

func TestTruncateLLMReplyNoOp(t *testing.T) {
	s := "hello мир"
	if got := TruncateLLMReply(s, 100); got != s {
		t.Fatalf("got %q want %q", got, s)
	}
}

func TestTruncateLLMReplyCutsRunes(t *testing.T) {
	s := strings.Repeat("Я", 50)
	got := TruncateLLMReply(s, 10)
	if !strings.Contains(got, "[GEN: ответ обрезан") {
		t.Fatal("expected marker")
	}

	if strings.Count(got, "Я") > 10 {
		t.Fatalf("too many runes: %q", got)
	}
}
