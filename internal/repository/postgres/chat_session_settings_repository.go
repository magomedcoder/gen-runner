package postgres

import (
	"context"
	"errors"

	"github.com/lib/pq"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type chatSessionSettingsRepository struct {
	db *gorm.DB
}

func NewChatSessionSettingsRepository(db *gorm.DB) domain.ChatSessionSettingsRepository {
	return &chatSessionSettingsRepository{db: db}
}

func (r *chatSessionSettingsRepository) GetBySessionID(ctx context.Context, sessionID int64) (*domain.ChatSessionSettings, error) {
	settings := &domain.ChatSessionSettings{SessionID: sessionID}
	var row model.Chat
	err := r.db.WithContext(ctx).Where("id = ?", sessionID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return settings, nil
		}
		return nil, err
	}
	return chatRowToSessionSettings(&row), nil
}

func (r *chatSessionSettingsRepository) Upsert(ctx context.Context, settings *domain.ChatSessionSettings) error {
	seq := pq.StringArray(settings.StopSequences)
	if seq == nil {
		seq = pq.StringArray{}
	}
	return r.db.WithContext(ctx).Model(&model.Chat{}).
		Where("id = ?", settings.SessionID).
		Updates(map[string]any{
			"system_prompt":   settings.SystemPrompt,
			"stop_sequences":  seq,
			"timeout_seconds": settings.TimeoutSeconds,
			"temperature":     settings.Temperature,
			"top_k":           settings.TopK,
			"top_p":           settings.TopP,
			"json_mode":       settings.JSONMode,
			"json_schema":     settings.JSONSchema,
			"tools_json":      settings.ToolsJSON,
			"profile":         settings.Profile,
			"updated_at":      gorm.Expr("NOW()"),
		}).Error
}

func chatRowToSessionSettings(m *model.Chat) *domain.ChatSessionSettings {
	var seq []string
	if m.StopSequences != nil {
		seq = []string(m.StopSequences)
	}
	return &domain.ChatSessionSettings{
		SessionID:      m.ID,
		SystemPrompt:   m.SystemPrompt,
		StopSequences:  seq,
		TimeoutSeconds: m.TimeoutSeconds,
		Temperature:    m.Temperature,
		TopK:           m.TopK,
		TopP:           m.TopP,
		JSONMode:       m.JSONMode,
		JSONSchema:     m.JSONSchema,
		ToolsJSON:      m.ToolsJSON,
		Profile:        m.Profile,
	}
}
