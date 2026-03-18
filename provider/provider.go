package provider

import (
	"context"
	"fmt"

	"github.com/magomedcoder/llm-runner/config"
	"github.com/magomedcoder/llm-runner/domain"
	"github.com/magomedcoder/llm-runner/service"
)

type TextBackend interface {
	CheckConnection(ctx context.Context) (bool, error)

	GetModels(ctx context.Context) ([]string, error)

	SendMessage(ctx context.Context, model string, messages []*domain.AIChatMessage, stopSequences []string, genParams *domain.GenerationParams) (chan string, error)

	Embed(ctx context.Context, model string, text string) ([]float32, error)
}

type TextProvider interface {
	CheckConnection(ctx context.Context) (bool, error)

	GetModels(ctx context.Context) ([]string, error)

	SendMessage(ctx context.Context, sessionId int64, model string, messages []*domain.AIChatMessage, stopSequences []string, genParams *domain.GenerationParams) (chan string, error)

	Embed(ctx context.Context, model string, text string) ([]float32, error)
}

func NewTextProvider(cfg *config.Config) (TextProvider, error) {
	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("задайте model_path")
	}

	var opts []service.LlamaOption
	if cfg.MaxContextTokens > 0 {
		opts = append(opts, service.WithMaxContextTokens(cfg.MaxContextTokens))
	}

	opts = append(opts, service.WithEmbeddings(true))
	svc := service.NewLlamaService(cfg.ModelPath, opts...)

	return NewText(svc), nil
}
