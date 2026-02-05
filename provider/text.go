package provider

import (
	"context"

	"github.com/magomedcoder/llm-runner/domain"
)

type Text struct {
	backend TextBackend
}

func NewText(backend TextBackend) *Text {
	return &Text{
		backend: backend,
	}
}

func (t *Text) CheckConnection(ctx context.Context) (bool, error) {
	return t.backend.CheckConnection(ctx)
}

func (t *Text) GetModels(ctx context.Context) ([]string, error) {
	return t.backend.GetModels(ctx)
}

func (t *Text) SendMessage(ctx context.Context, sessionId int64, model string, messages []*domain.AIChatMessage, stopSequences []string, genParams *domain.GenerationParams) (chan string, error) {
	return t.backend.SendMessage(ctx, model, messages, stopSequences, genParams)
}

func (t *Text) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	return t.backend.Embed(ctx, model, text)
}
