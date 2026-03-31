package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magomedcoder/gen/internal/domain"
)

type messageRepository struct {
	db *pgxpool.Pool
}

func NewMessageRepository(db *pgxpool.Pool) domain.MessageRepository {
	return &messageRepository{db: db}
}

func nullInt64Ptr(v *int64) interface{} {
	if v == nil {
		return nil
	}

	return *v
}

func nullTrimmedString(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	return s
}

func (r *messageRepository) Create(ctx context.Context, message *domain.Message) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO messages (session_id, content, role, attachment_file_id, tool_call_id, tool_name, tool_calls_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`,
		message.SessionId,
		message.Content,
		message.Role,
		nullInt64Ptr(message.AttachmentFileID),
		nullTrimmedString(message.ToolCallID),
		nullTrimmedString(message.ToolName),
		nullTrimmedString(message.ToolCallsJSON),
		message.CreatedAt,
		message.UpdatedAt,
	).Scan(&message.Id)

	return err
}

func (r *messageRepository) UpdateContent(ctx context.Context, id int64, content string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE messages
		SET content = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id, content)
	return err
}

func (r *messageRepository) GetBySessionId(ctx context.Context, sessionID int64, page, pageSize int32) ([]*domain.Message, int32, error) {
	_, pageSize, offset := normalizePagination(page, pageSize)

	var total int32
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM messages
		WHERE session_id = $1 AND deleted_at IS NULL
	`, sessionID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT m.id, m.session_id, m.content, m.role, m.attachment_file_id,
		       COALESCE(m.tool_call_id, ''), COALESCE(m.tool_name, ''), COALESCE(m.tool_calls_json, ''),
		       f.filename, m.created_at, m.updated_at, m.deleted_at
		FROM messages m
		LEFT JOIN files f ON m.attachment_file_id = f.id
		WHERE m.session_id = $1 AND m.deleted_at IS NULL
		ORDER BY m.created_at ASC
		LIMIT $2 OFFSET $3
	`, sessionID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var messages []*domain.Message
	for rows.Next() {
		var message domain.Message
		var attachmentFileID *int64
		var fname *string
		if err := rows.Scan(
			&message.Id,
			&message.SessionId,
			&message.Content,
			&message.Role,
			&attachmentFileID,
			&message.ToolCallID,
			&message.ToolName,
			&message.ToolCallsJSON,
			&fname,
			&message.CreatedAt,
			&message.UpdatedAt,
			&message.DeletedAt,
		); err != nil {
			return nil, 0, err
		}
		message.AttachmentFileID = attachmentFileID
		if fname != nil {
			message.AttachmentName = *fname
		}
		messages = append(messages, &message)
	}

	if rows.Err() != nil {
		return nil, 0, rows.Err()
	}

	return messages, total, nil
}

func (r *messageRepository) ListBySessionCreatedAtWindowIncludingDeleted(ctx context.Context, sessionID int64, fromInclusive, toExclusive time.Time) ([]*domain.Message, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.id, m.session_id, m.content, m.role, m.attachment_file_id,
		       COALESCE(m.tool_call_id, ''), COALESCE(m.tool_name, ''), COALESCE(m.tool_calls_json, ''),
		       f.filename, m.created_at, m.updated_at, m.deleted_at
		FROM messages m
		LEFT JOIN files f ON m.attachment_file_id = f.id
		WHERE m.session_id = $1 AND m.created_at >= $2 AND m.created_at < $3
		ORDER BY m.created_at ASC, m.id ASC
	`, sessionID, fromInclusive, toExclusive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*domain.Message
	for rows.Next() {
		var message domain.Message
		var attachmentFileID *int64
		var fname *string
		if err := rows.Scan(
			&message.Id,
			&message.SessionId,
			&message.Content,
			&message.Role,
			&attachmentFileID,
			&message.ToolCallID,
			&message.ToolName,
			&message.ToolCallsJSON,
			&fname,
			&message.CreatedAt,
			&message.UpdatedAt,
			&message.DeletedAt,
		); err != nil {
			return nil, err
		}
		message.AttachmentFileID = attachmentFileID
		if fname != nil {
			message.AttachmentName = *fname
		}
		messages = append(messages, &message)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return messages, nil
}

func (r *messageRepository) GetByID(ctx context.Context, id int64) (*domain.Message, error) {
	row := r.db.QueryRow(ctx, `
		SELECT m.id, m.session_id, m.content, m.role, m.attachment_file_id,
		       COALESCE(m.tool_call_id, ''), COALESCE(m.tool_name, ''), COALESCE(m.tool_calls_json, ''),
		       f.filename, m.created_at, m.updated_at, m.deleted_at
		FROM messages m
		LEFT JOIN files f ON m.attachment_file_id = f.id
		WHERE m.id = $1 AND m.deleted_at IS NULL
	`, id)

	var message domain.Message
	var attachmentFileID *int64
	var fname *string
	if err := row.Scan(
		&message.Id,
		&message.SessionId,
		&message.Content,
		&message.Role,
		&attachmentFileID,
		&message.ToolCallID,
		&message.ToolName,
		&message.ToolCallsJSON,
		&fname,
		&message.CreatedAt,
		&message.UpdatedAt,
		&message.DeletedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	message.AttachmentFileID = attachmentFileID
	if fname != nil {
		message.AttachmentName = *fname
	}

	return &message, nil
}

func (r *messageRepository) ListMessagesWithIDLessThan(ctx context.Context, sessionID int64, beforeMessageID int64) ([]*domain.Message, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.id, m.session_id, m.content, m.role, m.attachment_file_id,
		       COALESCE(m.tool_call_id, ''), COALESCE(m.tool_name, ''), COALESCE(m.tool_calls_json, ''),
		       f.filename, m.created_at, m.updated_at, m.deleted_at
		FROM messages m
		LEFT JOIN files f ON m.attachment_file_id = f.id
		WHERE m.session_id = $1 AND m.deleted_at IS NULL AND m.id < $2
		ORDER BY m.created_at ASC, m.id ASC
	`, sessionID, beforeMessageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*domain.Message
	for rows.Next() {
		var message domain.Message
		var attachmentFileID *int64
		var fname *string
		if err := rows.Scan(
			&message.Id,
			&message.SessionId,
			&message.Content,
			&message.Role,
			&attachmentFileID,
			&message.ToolCallID,
			&message.ToolName,
			&message.ToolCallsJSON,
			&fname,
			&message.CreatedAt,
			&message.UpdatedAt,
			&message.DeletedAt,
		); err != nil {
			return nil, err
		}

		message.AttachmentFileID = attachmentFileID
		if fname != nil {
			message.AttachmentName = *fname
		}

		messages = append(messages, &message)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return messages, nil
}

func (r *messageRepository) ListMessagesUpToID(ctx context.Context, sessionID int64, upToMessageID int64) ([]*domain.Message, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.id, m.session_id, m.content, m.role, m.attachment_file_id,
		       COALESCE(m.tool_call_id, ''), COALESCE(m.tool_name, ''), COALESCE(m.tool_calls_json, ''),
		       f.filename, m.created_at, m.updated_at, m.deleted_at
		FROM messages m
		LEFT JOIN files f ON m.attachment_file_id = f.id
		WHERE m.session_id = $1 AND m.deleted_at IS NULL AND m.id <= $2
		ORDER BY m.created_at ASC, m.id ASC
	`, sessionID, upToMessageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*domain.Message
	for rows.Next() {
		var message domain.Message
		var attachmentFileID *int64
		var fname *string
		if err := rows.Scan(
			&message.Id,
			&message.SessionId,
			&message.Content,
			&message.Role,
			&attachmentFileID,
			&message.ToolCallID,
			&message.ToolName,
			&message.ToolCallsJSON,
			&fname,
			&message.CreatedAt,
			&message.UpdatedAt,
			&message.DeletedAt,
		); err != nil {
			return nil, err
		}

		message.AttachmentFileID = attachmentFileID
		if fname != nil {
			message.AttachmentName = *fname
		}

		messages = append(messages, &message)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return messages, nil
}

func (r *messageRepository) SoftDeleteAfterID(ctx context.Context, sessionID int64, afterMessageID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE messages
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE session_id = $1 AND deleted_at IS NULL AND id > $2
	`, sessionID, afterMessageID)
	return err
}

func (r *messageRepository) SoftDeleteRangeAfterID(ctx context.Context, sessionID int64, afterMessageID int64, upToMessageID int64) error {
	if upToMessageID <= 0 {
		return r.SoftDeleteAfterID(ctx, sessionID, afterMessageID)
	}

	_, err := r.db.Exec(ctx, `
		UPDATE messages
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE session_id = $1 AND deleted_at IS NULL AND id > $2 AND id <= $3
	`, sessionID, afterMessageID, upToMessageID)

	return err
}

func (r *messageRepository) ResetAssistantForRegenerate(ctx context.Context, sessionID int64, messageID int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE messages
		SET content = $3,
		    tool_calls_json = NULL,
		    tool_call_id = NULL,
		    tool_name = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND session_id = $2 AND role = 'assistant' AND deleted_at IS NULL
	`, messageID, sessionID, "")
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return errors.New("сообщение не найдено или не является ответом ассистента")
	}

	return nil
}

func (r *messageRepository) MaxMessageIDInSession(ctx context.Context, sessionID int64) (int64, error) {
	var maxID *int64
	err := r.db.QueryRow(ctx, `
		SELECT MAX(id) FROM messages
		WHERE session_id = $1 AND deleted_at IS NULL
	`, sessionID).Scan(&maxID)
	if err != nil {
		return 0, err
	}

	if maxID == nil {
		return 0, nil
	}

	return *maxID, nil
}
