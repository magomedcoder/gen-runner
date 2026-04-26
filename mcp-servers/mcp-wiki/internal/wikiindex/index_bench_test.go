package wikiindex

import (
	"fmt"
	"testing"
)

func BenchmarkSearch_1kChunks(b *testing.B) {
	const n = 1000
	chunks := make([]InputChunk, n)
	for i := range n {
		chunks[i] = InputChunk{
			FilePath:   fmt.Sprintf("wiki/page/%d.md", i),
			FileName:   fmt.Sprintf("%d.md", i),
			ChunkIndex: 0,
			Text:       fmt.Sprintf("benchtoken%d alpha beta gamma delta epsilon", i),
		}
	}
	idx := New()
	idx.Build("/bench/wiki", chunks)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.Search("benchtoken500 alpha beta", 10)
	}
}
