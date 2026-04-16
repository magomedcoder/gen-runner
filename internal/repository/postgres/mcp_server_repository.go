package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type mcpServerRepository struct {
	db *gorm.DB
}

func NewMCPServerRepository(db *gorm.DB) domain.MCPServerRepository {
	return &mcpServerRepository{db: db}
}

func rowToMCPServer(m *model.MCPServer) *domain.MCPServer {
	if m == nil {
		return nil
	}
	return &domain.MCPServer{
		ID:             m.ID,
		UserID:         m.UserID,
		Name:           m.Name,
		Enabled:        m.Enabled,
		Transport:      m.Transport,
		Command:        m.Command,
		ArgsJSON:       m.ArgsJSON,
		EnvJSON:        m.EnvJSON,
		URL:            m.URL,
		HeadersJSON:    m.HeadersJSON,
		TimeoutSeconds: m.TimeoutSeconds,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func (r *mcpServerRepository) ListGlobal(ctx context.Context) ([]*domain.MCPServer, error) {
	var rows []model.MCPServer
	if err := r.db.WithContext(ctx).Where("user_id IS NULL").Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]*domain.MCPServer, 0, len(rows))
	for i := range rows {
		out = append(out, rowToMCPServer(&rows[i]))
	}

	return out, nil
}

func (r *mcpServerRepository) ListForUser(ctx context.Context, userID int) ([]*domain.MCPServer, error) {
	var rows []model.MCPServer
	if err := r.db.WithContext(ctx).
		Where("user_id IS NULL OR user_id = ?", userID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]*domain.MCPServer, 0, len(rows))
	for i := range rows {
		out = append(out, rowToMCPServer(&rows[i]))
	}

	return out, nil
}

func (r *mcpServerRepository) ListActive(ctx context.Context) ([]*domain.MCPServer, error) {
	var rows []model.MCPServer
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]*domain.MCPServer, 0, len(rows))
	for i := range rows {
		out = append(out, rowToMCPServer(&rows[i]))
	}

	return out, nil
}

func (r *mcpServerRepository) GetByID(ctx context.Context, id int64) (*domain.MCPServer, error) {
	var m model.MCPServer
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}

		return nil, err
	}

	return rowToMCPServer(&m), nil
}

func (r *mcpServerRepository) GetByIDAccessible(ctx context.Context, id int64, userID int) (*domain.MCPServer, error) {
	var m model.MCPServer
	err := r.db.WithContext(ctx).
		Where("id = ? AND (user_id IS NULL OR user_id = ?)", id, userID).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}

		return nil, err
	}

	return rowToMCPServer(&m), nil
}

func (r *mcpServerRepository) GetGlobalByID(ctx context.Context, id int64) (*domain.MCPServer, error) {
	var m model.MCPServer
	err := r.db.WithContext(ctx).Where("id = ? AND user_id IS NULL", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}

		return nil, err
	}

	return rowToMCPServer(&m), nil
}

func (r *mcpServerRepository) Create(ctx context.Context, s *domain.MCPServer) (*domain.MCPServer, error) {
	if s == nil {
		return nil, errors.New("nil mcp server")
	}

	m := model.MCPServer{
		UserID:         s.UserID,
		Name:           strings.TrimSpace(s.Name),
		Enabled:        s.Enabled,
		Transport:      normalizeMCPTransport(s.Transport),
		Command:        s.Command,
		ArgsJSON:       defaultJSONArray(s.ArgsJSON),
		EnvJSON:        defaultJSONObject(s.EnvJSON),
		URL:            strings.TrimSpace(s.URL),
		HeadersJSON:    defaultJSONObject(s.HeadersJSON),
		TimeoutSeconds: normalizeMCPTimeout(s.TimeoutSeconds),
	}

	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}

	return rowToMCPServer(&m), nil
}

func (r *mcpServerRepository) UpdateGlobal(ctx context.Context, s *domain.MCPServer) error {
	if s == nil || s.ID <= 0 {
		return errors.New("invalid mcp server")
	}

	res := r.db.WithContext(ctx).Model(&model.MCPServer{}).Where("id = ? AND user_id IS NULL", s.ID).Updates(map[string]any{
		"name":            strings.TrimSpace(s.Name),
		"enabled":         s.Enabled,
		"transport":       normalizeMCPTransport(s.Transport),
		"command":         s.Command,
		"args_json":       defaultJSONArray(s.ArgsJSON),
		"env_json":        defaultJSONObject(s.EnvJSON),
		"url":             strings.TrimSpace(s.URL),
		"headers_json":    defaultJSONObject(s.HeadersJSON),
		"timeout_seconds": normalizeMCPTimeout(s.TimeoutSeconds),
		"updated_at":      gorm.Expr("NOW()"),
	})
	if res.Error != nil {
		return res.Error
	}

	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *mcpServerRepository) UpdateOwned(ctx context.Context, s *domain.MCPServer, ownerUserID int) error {
	if s == nil || s.ID <= 0 {
		return errors.New("invalid mcp server")
	}

	res := r.db.WithContext(ctx).Model(&model.MCPServer{}).Where("id = ? AND user_id = ?", s.ID, ownerUserID).Updates(map[string]any{
		"name":            strings.TrimSpace(s.Name),
		"enabled":         s.Enabled,
		"transport":       normalizeMCPTransport(s.Transport),
		"command":         s.Command,
		"args_json":       defaultJSONArray(s.ArgsJSON),
		"env_json":        defaultJSONObject(s.EnvJSON),
		"url":             strings.TrimSpace(s.URL),
		"headers_json":    defaultJSONObject(s.HeadersJSON),
		"timeout_seconds": normalizeMCPTimeout(s.TimeoutSeconds),
		"updated_at":      gorm.Expr("NOW()"),
	})

	if res.Error != nil {
		return res.Error
	}

	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *mcpServerRepository) DeleteGlobal(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Where("id = ? AND user_id IS NULL", id).Delete(&model.MCPServer{})
	if res.Error != nil {
		return res.Error
	}

	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *mcpServerRepository) DeleteOwned(ctx context.Context, id int64, ownerUserID int) error {
	res := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, ownerUserID).Delete(&model.MCPServer{})
	if res.Error != nil {
		return res.Error
	}

	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *mcpServerRepository) CountOwnedByUser(ctx context.Context, userID int) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.MCPServer{}).Where("user_id = ?", userID).Count(&n).Error
	if err != nil {
		return 0, err
	}

	return n, nil
}

func normalizeMCPTransport(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "stdio", "sse", "streamable":
		return t
	default:
		return "stdio"
	}
}

func normalizeMCPTimeout(sec int32) int32 {
	if sec <= 0 {
		return 120
	}

	if sec > 600 {
		return 600
	}

	return sec
}

func defaultJSONArray(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "[]"
	}

	return s
}

func defaultJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "{}"
	}

	return s
}
