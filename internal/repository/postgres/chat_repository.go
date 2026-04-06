package postgres

import (
	"context"
	"time"

	"github.com/lib/pq"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type chatSessionRepository struct {
	db *gorm.DB
}

func NewChatSessionRepository(db *gorm.DB) domain.ChatSessionRepository {
	return &chatSessionRepository{db: db}
}

func (r *chatSessionRepository) Create(ctx context.Context, session *domain.ChatSession) error {
	selectedRunner := ""
	var prev model.Chat
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", session.UserId).
		Order("updated_at DESC").
		First(&prev).Error; err == nil {
		selectedRunner = prev.SelectedRunner
	}

	row := model.Chat{
		UserID:         session.UserId,
		Title:          session.Title,
		Model:          session.Model,
		SelectedRunner: selectedRunner,
		StopSequences:  pq.StringArray{},
		CreatedAt:      session.CreatedAt,
		UpdatedAt:      session.UpdatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	session.Id = row.ID
	return nil
}

func (r *chatSessionRepository) GetById(ctx context.Context, id int64) (*domain.ChatSession, error) {
	var row model.Chat
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&row).Error
	if err != nil {
		return nil, handleNotFound(err, "сессия не найдена")
	}
	return chatToDomain(&row), nil
}

func (r *chatSessionRepository) GetByUserId(ctx context.Context, userID int, page, pageSize int32) ([]*domain.ChatSession, int32, error) {
	_, pageSize, offset := normalizePagination(page, pageSize)

	var total int64
	if err := r.db.WithContext(ctx).Model(&model.Chat{}).
		Scopes(scopeChatUser(userID)).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []model.Chat
	if err := r.db.WithContext(ctx).Model(&model.Chat{}).
		Scopes(scopeChatUser(userID)).
		Order("created_at DESC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	out := make([]*domain.ChatSession, 0, len(rows))
	for i := range rows {
		out = append(out, chatToDomain(&rows[i]))
	}
	return out, int32(total), nil
}

func (r *chatSessionRepository) Update(ctx context.Context, session *domain.ChatSession) error {
	session.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Model(&model.Chat{}).
		Where("id = ?", session.Id).
		Updates(map[string]any{
			"title":      session.Title,
			"model":      session.Model,
			"updated_at": session.UpdatedAt,
		}).Error
}

func (r *chatSessionRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&model.Chat{}).Error
}

func chatToDomain(m *model.Chat) *domain.ChatSession {
	return &domain.ChatSession{
		Id:        m.ID,
		UserId:    m.UserID,
		Title:     m.Title,
		Model:     m.Model,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
		DeletedAt: gormDeletedAtToPtr(m.DeletedAt),
	}
}
