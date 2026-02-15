package repository

import (
	"context"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/service"
)

type LLMRunnerRepository struct {
	client *service.LLMRunnerService
}

func NewLLMRunnerRepository(address, model string) (*LLMRunnerRepository, error) {
	client, err := service.NewLLMRunnerService(address, model)
	if err != nil {
		return nil, err
	}
	return &LLMRunnerRepository{client: client}, nil
}

func (r *LLMRunnerRepository) CheckConnection(ctx context.Context) (bool, error) {
	return r.client.CheckConnection(ctx)
}

func (r *LLMRunnerRepository) GetModels(ctx context.Context) ([]string, error) {
	return r.client.GetModels(ctx)
}

func (r *LLMRunnerRepository) SendMessage(ctx context.Context, sessionID string, model string, messages []*domain.Message) (chan string, error) {
	return r.client.SendMessage(ctx, sessionID, model, messages)
}

func (r *LLMRunnerRepository) Close() error {
	return r.client.Close()
}
