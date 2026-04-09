package usecase

import (
	"context"
	"fmt"
)

type embedRunner interface {
	Embed(ctx context.Context, model string, text string) ([]float32, error)

	EmbedBatch(ctx context.Context, model string, texts []string) ([][]float32, error)
}

func embedTextsBatches(ctx context.Context, llm embedRunner, model string, texts []string, batchSize int) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	if batchSize <= 0 {
		batchSize = 32
	}

	var out [][]float32
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		part, err := embedTextsRecursive(ctx, llm, model, texts[i:end])
		if err != nil {
			return nil, err
		}

		out = append(out, part...)
	}

	if len(out) != len(texts) {
		return nil, fmt.Errorf("эмбеддинги: ожидалось %d векторов, получено %d", len(texts), len(out))
	}

	return out, nil
}

func embedTextsRecursive(ctx context.Context, llm embedRunner, model string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	if len(texts) == 1 {
		v, err := llm.Embed(ctx, model, texts[0])
		if err != nil {
			return nil, err
		}

		if len(v) == 0 {
			return nil, fmt.Errorf("эмбеддинги: пустой вектор для одного чанка")
		}

		return [][]float32{v}, nil
	}

	batch, err := llm.EmbedBatch(ctx, model, texts)
	if err != nil {
		return splitAndEmbed(ctx, llm, model, texts)
	}

	if len(batch) != len(texts) {
		return splitAndEmbed(ctx, llm, model, texts)
	}

	for i := range batch {
		if len(batch[i]) == 0 {
			one, err := embedTextsRecursive(ctx, llm, model, texts[i:i+1])
			if err != nil {
				return nil, err
			}

			batch[i] = one[0]
		}
	}

	return batch, nil
}

func splitAndEmbed(ctx context.Context, llm embedRunner, model string, texts []string) ([][]float32, error) {
	if len(texts) == 1 {
		return embedTextsRecursive(ctx, llm, model, texts)
	}

	mid := len(texts) / 2
	if mid == 0 {
		mid = 1
	}

	a, err := embedTextsRecursive(ctx, llm, model, texts[:mid])
	if err != nil {
		return nil, err
	}

	b, err := embedTextsRecursive(ctx, llm, model, texts[mid:])
	if err != nil {
		return nil, err
	}

	return append(a, b...), nil
}
