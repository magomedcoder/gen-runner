package mcpclient

import (
	"strings"
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
)

func BenchmarkTruncateLLMReply_noopUnderLimit(b *testing.B) {
	s := strings.Repeat("a", 10_000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TruncateLLMReply(s, MaxMetaToolReplyRunes)
	}
}

func BenchmarkTruncateLLMReply_truncateLargeASCII(b *testing.B) {
	s := strings.Repeat("x", MaxMetaToolReplyRunes*2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TruncateLLMReply(s, MaxMetaToolReplyRunes)
	}
}

func BenchmarkTruncateLLMReply_truncateCyrillicRunes(b *testing.B) {
	chunk := strings.Repeat("Я", 500)
	s := strings.Repeat(chunk, (MaxMetaToolReplyRunes*2)/500+2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TruncateLLMReply(s, MaxMetaToolReplyRunes)
	}
}

func BenchmarkServerConfigFingerprint(b *testing.B) {
	srv := &domain.MCPServer{
		ID:             42,
		Transport:      "streamable",
		Command:        "/usr/bin/node",
		ArgsJSON:       `["dist/main.js"]`,
		EnvJSON:        `{"NODE_ENV":"production"}`,
		URL:            "https://example.com/mcp",
		HeadersJSON:    `{"Authorization":"Bearer x"}`,
		TimeoutSeconds: 180,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = serverConfigFingerprint(srv)
	}
}

func BenchmarkMCPCountersMap(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MCPCountersMap()
	}
}
