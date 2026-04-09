package usecase

import (
	"context"
	"errors"
	"testing"
)

type stubEmbed struct {
	failBatchLen int
	calls        int
}

func (s *stubEmbed) Embed(_ context.Context, _ string, text string) ([]float32, error) {
	s.calls++
	return []float32{float32(len(text))}, nil
}

func (s *stubEmbed) EmbedBatch(_ context.Context, _ string, texts []string) ([][]float32, error) {
	s.calls++
	if s.failBatchLen > 0 && len(texts) > s.failBatchLen {
		return nil, errors.New("batch too large")
	}

	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{float32(i)}
	}

	return out, nil
}

func TestEmbedTextsBatches_recursiveOnBatchError(t *testing.T) {
	ctx := context.Background()
	st := &stubEmbed{failBatchLen: 2}
	texts := []string{"a", "b", "c", "d"}
	vec, err := embedTextsBatches(ctx, st, "m", texts, 4)
	if err != nil {
		t.Fatal(err)
	}

	if len(vec) != 4 {
		t.Fatalf("len %d", len(vec))
	}

	if st.calls < 3 {
		t.Fatalf("expected several embed calls, got %d", st.calls)
	}
}

func TestEmbedTextsRecursive_mismatchedBatchLengthSplits(t *testing.T) {
	ctx := context.Background()
	llm := &badCountEmbed{}
	vec, err := embedTextsRecursive(ctx, llm, "m", []string{"x", "y"})
	if err != nil {
		t.Fatal(err)
	}

	if len(vec) != 2 {
		t.Fatalf("len %d", len(vec))
	}
}

type badCountEmbed struct{}

func (b *badCountEmbed) Embed(context.Context, string, string) ([]float32, error) {
	return []float32{1}, nil
}

func (b *badCountEmbed) EmbedBatch(_ context.Context, _ string, texts []string) ([][]float32, error) {
	if len(texts) == 2 {
		return [][]float32{{1}}, nil
	}

	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1}
	}

	return out, nil
}
