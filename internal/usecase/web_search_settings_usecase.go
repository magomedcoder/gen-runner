package usecase

import (
	"context"

	"github.com/magomedcoder/gen/internal/domain"
)

type WebSearchSettingsUseCase struct {
	repo domain.WebSearchSettingsRepository
}

func NewWebSearchSettingsUseCase(repo domain.WebSearchSettingsRepository) *WebSearchSettingsUseCase {
	return &WebSearchSettingsUseCase{repo: repo}
}

func normalizeWebSearchMaxResults(n int) int {
	if n <= 0 {
		return 20
	}

	if n > 50 {
		return 50
	}

	return n
}

func (u *WebSearchSettingsUseCase) Get(ctx context.Context) (*domain.WebSearchSettings, error) {
	return u.repo.Get(ctx)
}

func (u *WebSearchSettingsUseCase) Update(ctx context.Context, s *domain.WebSearchSettings) error {
	if s == nil {
		return nil
	}

	s.MaxResults = normalizeWebSearchMaxResults(s.MaxResults)

	return u.repo.Upsert(ctx, s)
}
