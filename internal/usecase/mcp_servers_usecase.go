package usecase

import (
	"context"

	"github.com/magomedcoder/gen/internal/domain"
)

type MCPServersUseCase struct {
	repo domain.MCPServerRepository
}

func NewMCPServersUseCase(repo domain.MCPServerRepository) *MCPServersUseCase {
	return &MCPServersUseCase{repo: repo}
}

func (u *MCPServersUseCase) ListGlobal(ctx context.Context) ([]*domain.MCPServer, error) {
	if u.repo == nil {
		return nil, nil
	}

	return u.repo.ListGlobal(ctx)
}

func (u *MCPServersUseCase) ListForUser(ctx context.Context, userID int) ([]*domain.MCPServer, error) {
	if u.repo == nil {
		return nil, nil
	}

	return u.repo.ListForUser(ctx, userID)
}

func (u *MCPServersUseCase) ListActive(ctx context.Context) ([]*domain.MCPServer, error) {
	if u.repo == nil {
		return nil, nil
	}

	return u.repo.ListActive(ctx)
}

func (u *MCPServersUseCase) GetGlobal(ctx context.Context, id int64) (*domain.MCPServer, error) {
	return u.repo.GetGlobalByID(ctx, id)
}

func (u *MCPServersUseCase) GetForUser(ctx context.Context, id int64, userID int) (*domain.MCPServer, error) {
	return u.repo.GetByIDAccessible(ctx, id, userID)
}

func (u *MCPServersUseCase) CreateGlobal(ctx context.Context, s *domain.MCPServer) (*domain.MCPServer, error) {
	if s == nil {
		return nil, nil
	}

	s.UserID = nil
	return u.repo.Create(ctx, s)
}

func (u *MCPServersUseCase) CreateOwned(ctx context.Context, s *domain.MCPServer, userID int) (*domain.MCPServer, error) {
	if s == nil {
		return nil, nil
	}

	uid := userID
	s.UserID = &uid
	return u.repo.Create(ctx, s)
}

func (u *MCPServersUseCase) UpdateGlobal(ctx context.Context, s *domain.MCPServer) error {
	return u.repo.UpdateGlobal(ctx, s)
}

func (u *MCPServersUseCase) UpdateOwned(ctx context.Context, s *domain.MCPServer, userID int) error {
	return u.repo.UpdateOwned(ctx, s, userID)
}

func (u *MCPServersUseCase) DeleteGlobal(ctx context.Context, id int64) error {
	return u.repo.DeleteGlobal(ctx, id)
}

func (u *MCPServersUseCase) DeleteOwned(ctx context.Context, id int64, userID int) error {
	return u.repo.DeleteOwned(ctx, id, userID)
}

func (u *MCPServersUseCase) CountOwnedByUser(ctx context.Context, userID int) (int64, error) {
	if u.repo == nil {
		return 0, nil
	}

	return u.repo.CountOwnedByUser(ctx, userID)
}
