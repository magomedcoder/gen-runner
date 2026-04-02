package postgres

import (
	"context"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type assistantMessageRegenerationRepository struct {
	db *gorm.DB
}

func NewAssistantMessageRegenerationRepository(db *gorm.DB) domain.AssistantMessageRegenerationRepository {
	return &assistantMessageRegenerationRepository{db: db}
}

func (r *assistantMessageRegenerationRepository) Create(ctx context.Context, regen *domain.AssistantMessageRegeneration) error {
	if regen == nil {
		return nil
	}
	row := model.MessageEdit{
		SessionID:    regen.SessionId,
		MessageID:    regen.MessageId,
		EditorUserID: regen.RegenUserId,
		Kind:         "assistant_regen",
		OldContent:   regen.OldContent,
		NewContent:   regen.NewContent,
		CreatedAt:    regen.CreatedAt,
	}
	if err := r.db.WithContext(ctx).
		Omit("SoftDeletedFromID", "SoftDeletedToID", "RevertedAt").
		Create(&row).Error; err != nil {
		return err
	}
	regen.Id = row.ID
	return nil
}

func (r *assistantMessageRegenerationRepository) ListByMessageID(ctx context.Context, messageID int64, limit int32) ([]*domain.AssistantMessageRegeneration, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []model.MessageEdit
	if err := r.db.WithContext(ctx).
		Select("id", "session_id", "message_id", "editor_user_id", "old_content", "new_content", "created_at").
		Where("message_id = ? AND kind = ?", messageID, "assistant_regen").
		Order("created_at DESC, id DESC").
		Limit(int(limit)).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.AssistantMessageRegeneration, 0, len(rows))
	for i := range rows {
		out = append(out, &domain.AssistantMessageRegeneration{
			Id:          rows[i].ID,
			SessionId:   rows[i].SessionID,
			MessageId:   rows[i].MessageID,
			RegenUserId: rows[i].EditorUserID,
			OldContent:  rows[i].OldContent,
			NewContent:  rows[i].NewContent,
			CreatedAt:   rows[i].CreatedAt,
		})
	}
	return out, nil
}
