package postgres

import (
	"context"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type messageEditRepository struct {
	db *gorm.DB
}

func NewMessageEditRepository(db *gorm.DB) domain.MessageEditRepository {
	return &messageEditRepository{db: db}
}

func (r *messageEditRepository) Create(ctx context.Context, edit *domain.MessageEdit) error {
	row := model.MessageEdit{
		SessionID:         edit.SessionId,
		MessageID:         edit.MessageId,
		EditorUserID:      edit.EditorUserId,
		Kind:              "user_edit",
		OldContent:        edit.OldContent,
		NewContent:        edit.NewContent,
		SoftDeletedFromID: softDelPtr(edit.SoftDeletedFrom),
		SoftDeletedToID:   softDelPtr(edit.SoftDeletedTo),
		CreatedAt:         edit.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Omit("RevertedAt").Create(&row).Error; err != nil {
		return err
	}
	edit.Id = row.ID
	return nil
}

func softDelPtr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func (r *messageEditRepository) ListByMessageID(ctx context.Context, messageID int64, limit int32) ([]*domain.MessageEdit, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []model.MessageEdit
	if err := r.db.WithContext(ctx).
		Where("message_id = ? AND kind = ?", messageID, "user_edit").
		Order("created_at DESC, id DESC").
		Limit(int(limit)).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.MessageEdit, 0, len(rows))
	for i := range rows {
		out = append(out, messageEditToDomain(&rows[i]))
	}
	return out, nil
}

func messageEditToDomain(m *model.MessageEdit) *domain.MessageEdit {
	e := &domain.MessageEdit{
		Id:           m.ID,
		SessionId:    m.SessionID,
		MessageId:    m.MessageID,
		EditorUserId: m.EditorUserID,
		OldContent:   m.OldContent,
		NewContent:   m.NewContent,
		CreatedAt:    m.CreatedAt,
		RevertedAt:   m.RevertedAt,
	}
	if m.SoftDeletedFromID != nil {
		e.SoftDeletedFrom = *m.SoftDeletedFromID
	}
	if m.SoftDeletedToID != nil {
		e.SoftDeletedTo = *m.SoftDeletedToID
	}
	return e
}
