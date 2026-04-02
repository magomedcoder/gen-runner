package model

import "time"

type MessageEdit struct {
	ID                int64      `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID         int64      `gorm:"column:session_id"`
	MessageID         int64      `gorm:"column:message_id"`
	EditorUserID      int        `gorm:"column:editor_user_id"`
	Kind              string     `gorm:"column:kind"`
	OldContent        string     `gorm:"column:old_content"`
	NewContent        string     `gorm:"column:new_content"`
	SoftDeletedFromID *int64     `gorm:"column:soft_deleted_from_id"`
	SoftDeletedToID   *int64     `gorm:"column:soft_deleted_to_id"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	RevertedAt        *time.Time `gorm:"column:reverted_at"`
}

func (MessageEdit) TableName() string {
	return "message_edits"
}
