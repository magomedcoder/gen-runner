package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type messageRepository struct {
	db *gorm.DB
}

func NewMessageRepository(db *gorm.DB) domain.MessageRepository {
	return &messageRepository{db: db}
}

func preloadMessageAttachment(db *gorm.DB) *gorm.DB {
	return db.Select("id", "filename")
}

func (r *messageRepository) withAttachment(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Preload("Attachment", preloadMessageAttachment)
}

func messageModelToDomain(m *model.Message) *domain.Message {
	msg := &domain.Message{
		Id:               m.ID,
		SessionId:        m.SessionID,
		Content:          m.Content,
		Role:             domain.MessageRole(m.Role),
		AttachmentFileID: m.AttachmentFileID,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
		DeletedAt:        gormDeletedAtToPtr(m.DeletedAt),
	}
	if m.ToolCallID != nil {
		msg.ToolCallID = *m.ToolCallID
	}
	if m.ToolName != nil {
		msg.ToolName = *m.ToolName
	}
	if m.ToolCallsJSON != nil {
		msg.ToolCallsJSON = *m.ToolCallsJSON
	}
	if m.Attachment != nil {
		msg.AttachmentName = m.Attachment.Filename
	}
	return msg
}

func ptrTrimmedString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func (r *messageRepository) Create(ctx context.Context, message *domain.Message) error {
	row := model.Message{
		SessionID:        message.SessionId,
		Content:          message.Content,
		Role:             string(message.Role),
		AttachmentFileID: message.AttachmentFileID,
		ToolCallID:       ptrTrimmedString(message.ToolCallID),
		ToolName:         ptrTrimmedString(message.ToolName),
		ToolCallsJSON:    ptrTrimmedString(message.ToolCallsJSON),
		CreatedAt:        message.CreatedAt,
		UpdatedAt:        message.UpdatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	message.Id = row.ID
	return nil
}

func (r *messageRepository) UpdateContent(ctx context.Context, id int64, content string) error {
	return r.db.WithContext(ctx).Model(&model.Message{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"content":    content,
			"updated_at": gorm.Expr("NOW()"),
		}).Error
}

func (r *messageRepository) GetBySessionId(ctx context.Context, sessionID int64, page, pageSize int32) ([]*domain.Message, int32, error) {
	_, pageSize, offset := normalizePagination(page, pageSize)

	var total int64
	if err := r.db.WithContext(ctx).Model(&model.Message{}).
		Scopes(scopeMessageSession(sessionID)).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []model.Message
	err := r.withAttachment(ctx).
		Scopes(scopeMessageSession(sessionID)).
		Order("created_at ASC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	out := make([]*domain.Message, 0, len(rows))
	for i := range rows {
		out = append(out, messageModelToDomain(&rows[i]))
	}
	return out, int32(total), nil
}

func (r *messageRepository) ListBySessionBeforeID(ctx context.Context, sessionID int64, beforeMessageID int64, limit int32) ([]*domain.Message, int32, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := r.withAttachment(ctx).
		Scopes(scopeMessageSession(sessionID)).
		Order("id DESC").
		Limit(int(limit))
	if beforeMessageID > 0 {
		q = q.Where("id < ?", beforeMessageID)
	}

	var rows []model.Message
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}

	out := make([]*domain.Message, 0, len(rows))
	for i := range rows {
		out = append(out, messageModelToDomain(&rows[i]))
	}
	return out, 0, nil
}

func (r *messageRepository) SessionHasOlderMessages(ctx context.Context, sessionID int64, olderThanMessageID int64) (bool, error) {
	if olderThanMessageID <= 0 {
		return false, nil
	}
	var row model.Message
	err := r.db.WithContext(ctx).Model(&model.Message{}).
		Scopes(scopeMessageSession(sessionID)).
		Where("id < ?", olderThanMessageID).
		Limit(1).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *messageRepository) ListBySessionCreatedAtWindowIncludingDeleted(ctx context.Context, sessionID int64, fromInclusive, toExclusive time.Time) ([]*domain.Message, error) {
	var rows []model.Message
	err := r.db.WithContext(ctx).Unscoped().
		Preload("Attachment", preloadMessageAttachment).
		Scopes(scopeMessageSession(sessionID)).
		Where("created_at >= ? AND created_at < ?", fromInclusive, toExclusive).
		Order("created_at ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Message, 0, len(rows))
	for i := range rows {
		out = append(out, messageModelToDomain(&rows[i]))
	}
	return out, nil
}

func (r *messageRepository) GetByID(ctx context.Context, id int64) (*domain.Message, error) {
	var m model.Message
	err := r.withAttachment(ctx).Where("id = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return messageModelToDomain(&m), nil
}

func (r *messageRepository) ListMessagesWithIDLessThan(ctx context.Context, sessionID int64, beforeMessageID int64) ([]*domain.Message, error) {
	var rows []model.Message
	err := r.withAttachment(ctx).
		Scopes(scopeMessageSession(sessionID)).
		Where("id < ?", beforeMessageID).
		Order("created_at ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Message, 0, len(rows))
	for i := range rows {
		out = append(out, messageModelToDomain(&rows[i]))
	}
	return out, nil
}

func (r *messageRepository) ListMessagesUpToID(ctx context.Context, sessionID int64, upToMessageID int64) ([]*domain.Message, error) {
	var rows []model.Message
	err := r.withAttachment(ctx).
		Scopes(scopeMessageSession(sessionID)).
		Where("id <= ?", upToMessageID).
		Order("created_at ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Message, 0, len(rows))
	for i := range rows {
		out = append(out, messageModelToDomain(&rows[i]))
	}
	return out, nil
}

func (r *messageRepository) SoftDeleteAfterID(ctx context.Context, sessionID int64, afterMessageID int64) error {
	return r.db.WithContext(ctx).Scopes(scopeMessageSession(sessionID)).
		Where("id > ?", afterMessageID).
		Delete(&model.Message{}).Error
}

func (r *messageRepository) SoftDeleteRangeAfterID(ctx context.Context, sessionID int64, afterMessageID int64, upToMessageID int64) error {
	if upToMessageID <= 0 {
		return r.SoftDeleteAfterID(ctx, sessionID, afterMessageID)
	}
	return r.db.WithContext(ctx).Scopes(scopeMessageSession(sessionID)).
		Where("id > ? AND id <= ?", afterMessageID, upToMessageID).
		Delete(&model.Message{}).Error
}

func (r *messageRepository) ResetAssistantForRegenerate(ctx context.Context, sessionID int64, messageID int64) error {
	res := r.db.WithContext(ctx).Model(&model.Message{}).
		Where("id = ? AND session_id = ? AND role = ?", messageID, sessionID, "assistant").
		Updates(map[string]interface{}{
			"content":         "",
			"tool_calls_json": nil,
			"tool_call_id":    nil,
			"tool_name":       nil,
			"updated_at":      gorm.Expr("NOW()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("сообщение не найдено или не является ответом ассистента")
	}
	return nil
}

func (r *messageRepository) MaxMessageIDInSession(ctx context.Context, sessionID int64) (int64, error) {
	var maxID *int64
	err := r.db.WithContext(ctx).Model(&model.Message{}).
		Scopes(scopeMessageSession(sessionID)).
		Select("MAX(id)").
		Scan(&maxID).Error
	if err != nil {
		return 0, err
	}
	if maxID == nil {
		return 0, nil
	}
	return *maxID, nil
}
