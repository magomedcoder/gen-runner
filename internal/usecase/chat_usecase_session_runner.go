package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
)

func (c *ChatUseCase) GetSelectedRunner(ctx context.Context, userID int) (string, error) {
	s, err := c.preferenceRepo.GetSelectedRunner(ctx, userID)
	if err != nil {
		return "", err
	}

	s = strings.TrimSpace(s)
	if s != "" {
		return s, nil
	}

	if c.runnerReg != nil {
		if a := c.runnerReg.DefaultRunnerListenAddress(); a != "" {
			return a, nil
		}
	}

	return "", nil
}

func (c *ChatUseCase) SetSelectedRunner(ctx context.Context, userID int, runner string) error {
	return c.preferenceRepo.SetSelectedRunner(ctx, userID, runner)
}

func (c *ChatUseCase) verifySessionOwnership(ctx context.Context, userId int, sessionID int64) (*domain.ChatSession, error) {
	session, err := c.sessionRepo.GetById(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if session.UserId != userId {
		return nil, domain.ErrUnauthorized
	}

	return session, nil
}

func (c *ChatUseCase) chatRunnerAddrAndModel(ctx context.Context, session *domain.ChatSession) (addr string, model string, err error) {
	if session == nil || session.SelectedRunnerID == nil || *session.SelectedRunnerID <= 0 {
		return "", "", fmt.Errorf("сессии не назначен раннер")
	}

	ru, err := c.runnerRepo.GetByID(ctx, *session.SelectedRunnerID)
	if err != nil {
		return "", "", err
	}

	if !ru.Enabled {
		return "", "", fmt.Errorf("раннер для этого чата отключён")
	}

	model = strings.TrimSpace(ru.SelectedModel)
	if model == "" {
		return "", "", fmt.Errorf("у раннера чата не задана модель")
	}

	addr = domain.RunnerListenAddress(ru.Host, ru.Port)
	if addr == "" {
		return "", "", fmt.Errorf("некорректный адрес раннера")
	}

	return addr, model, nil
}

func (c *ChatUseCase) GetModels(ctx context.Context) ([]string, error) {
	return c.llmRepo.GetModels(ctx)
}

func (c *ChatUseCase) Embed(ctx context.Context, userID int, requestedModel string, text string) ([]float32, error) {
	model, err := resolveModelForUser(ctx, c.llmRepo, strings.TrimSpace(requestedModel), "")
	if err != nil {
		return nil, err
	}

	return c.llmRepo.Embed(ctx, model, text)
}

func (c *ChatUseCase) EmbedBatch(ctx context.Context, userID int, requestedModel string, texts []string) ([][]float32, error) {
	model, err := resolveModelForUser(ctx, c.llmRepo, strings.TrimSpace(requestedModel), "")
	if err != nil {
		return nil, err
	}

	return c.llmRepo.EmbedBatch(ctx, model, texts)
}
