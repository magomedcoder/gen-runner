package postgres

import (
	"context"

	"github.com/lib/pq"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type chatSessionSettingsRepository struct {
	db *gorm.DB
}

func NewChatSessionSettingsRepository(db *gorm.DB) domain.ChatSessionSettingsRepository {
	return &chatSessionSettingsRepository{db: db}
}

func (r *chatSessionSettingsRepository) GetBySessionID(ctx context.Context, sessionID int64) (*domain.ChatSessionSettings, error) {
	settings := &domain.ChatSessionSettings{SessionID: sessionID}
	var row model.ChatSetting
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&row).Error
	if err != nil {
		return settings, nil
	}
	return chatSettingRowToDomain(&row), nil
}

func (r *chatSessionSettingsRepository) Upsert(ctx context.Context, settings *domain.ChatSessionSettings) error {
	seq := pq.StringArray(settings.StopSequences)
	if seq == nil {
		seq = pq.StringArray{}
	}
	row := model.ChatSetting{
		SessionID:      settings.SessionID,
		SystemPrompt:   settings.SystemPrompt,
		StopSequences:  seq,
		TimeoutSeconds: settings.TimeoutSeconds,
		Temperature:    settings.Temperature,
		TopK:           settings.TopK,
		TopP:           settings.TopP,
		JSONMode:       settings.JSONMode,
		JSONSchema:     settings.JSONSchema,
		ToolsJSON:      settings.ToolsJSON,
		Profile:        settings.Profile,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"system_prompt":   gorm.Expr("EXCLUDED.system_prompt"),
			"stop_sequences":  gorm.Expr("EXCLUDED.stop_sequences"),
			"timeout_seconds": gorm.Expr("EXCLUDED.timeout_seconds"),
			"temperature":     gorm.Expr("EXCLUDED.temperature"),
			"top_k":           gorm.Expr("EXCLUDED.top_k"),
			"top_p":           gorm.Expr("EXCLUDED.top_p"),
			"json_mode":       gorm.Expr("EXCLUDED.json_mode"),
			"json_schema":     gorm.Expr("EXCLUDED.json_schema"),
			"tools_json":      gorm.Expr("EXCLUDED.tools_json"),
			"profile":         gorm.Expr("EXCLUDED.profile"),
			"updated_at":      gorm.Expr("NOW()"),
		}),
	}).Create(&row).Error
}

func chatSettingRowToDomain(m *model.ChatSetting) *domain.ChatSessionSettings {
	var seq []string
	if m.StopSequences != nil {
		seq = []string(m.StopSequences)
	}
	return &domain.ChatSessionSettings{
		SessionID:      m.SessionID,
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
