package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magomedcoder/gen/internal/domain"
)

type messageEditRepository struct {
	db *pgxpool.Pool
}

func NewMessageEditRepository(db *pgxpool.Pool) domain.MessageEditRepository {
	return &messageEditRepository{db: db}
}

func (r *messageEditRepository) Create(ctx context.Context, edit *domain.MessageEdit) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO message_edits (
			session_id, message_id, editor_user_id,
			old_content, new_content,
			soft_deleted_from_id, soft_deleted_to_id,
			created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id
	`,
		edit.SessionId,
		edit.MessageId,
		edit.EditorUserId,
		edit.OldContent,
		edit.NewContent,
		edit.SoftDeletedFrom,
		edit.SoftDeletedTo,
		edit.CreatedAt,
	).Scan(&edit.Id)
}

func (r *messageEditRepository) ListByMessageID(ctx context.Context, messageID int64, limit int32) ([]*domain.MessageEdit, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, session_id, message_id, editor_user_id,
		       old_content, new_content,
		       soft_deleted_from_id, soft_deleted_to_id,
		       created_at, reverted_at
		FROM message_edits
		WHERE message_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, messageID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.MessageEdit
	for rows.Next() {
		var e domain.MessageEdit
		if err := rows.Scan(
			&e.Id,
			&e.SessionId,
			&e.MessageId,
			&e.EditorUserId,
			&e.OldContent,
			&e.NewContent,
			&e.SoftDeletedFrom,
			&e.SoftDeletedTo,
			&e.CreatedAt,
			&e.RevertedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
