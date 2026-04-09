package rag

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitText_paragraphs(t *testing.T) {
	text := "Первый абзац.\n\nВторой абзац с текстом."
	chunks := SplitText("a.txt", text, SplitOptions{ChunkSizeRunes: 200, ChunkOverlapRunes: 0})
	if len(chunks) < 1 {
		t.Fatal("expected chunks")
	}

	joined := strings.Join(chunkTexts(chunks), " ")
	if !strings.Contains(joined, "Первый") || !strings.Contains(joined, "Второй") {
		t.Fatalf("unexpected join: %q", joined)
	}

	if chunks[0].Metadata["file_name"] != "a.txt" {
		t.Fatalf("metadata: %+v", chunks[0].Metadata)
	}
}

func TestSplitText_smallChunkForcesSplit(t *testing.T) {
	s := strings.Repeat("а", 50)
	chunks := SplitText("", s, SplitOptions{ChunkSizeRunes: 20, ChunkOverlapRunes: 0})
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	total := 0
	for _, c := range chunks {
		total += utf8.RuneCountInString(c.Text)
	}

	if total < 50 {
		t.Fatalf("lost runes: total=%d", total)
	}
}

func TestSplitText_overlap(t *testing.T) {
	text := strings.Repeat("word ", 100)
	chunks := SplitText("f.md", text, SplitOptions{ChunkSizeRunes: 80, ChunkOverlapRunes: 20})
	if len(chunks) < 2 {
		t.Fatalf("need 2+ chunks, got %d", len(chunks))
	}

	if _, ok := chunks[1].Metadata["chunk_index"]; !ok {
		t.Fatalf("expected chunk_index in metadata: %+v", chunks[1].Metadata)
	}
}

func chunkTexts(ch []Chunk) []string {
	out := make([]string, len(ch))
	for i := range ch {
		out[i] = ch[i].Text
	}

	return out
}
