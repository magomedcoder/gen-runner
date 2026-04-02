package model

import (
	"time"

	"gorm.io/gorm"
)

type Message struct {
	ID               int64          `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID        int64          `gorm:"column:session_id"`
	Content          string         `gorm:"column:content"`
	Role             string         `gorm:"column:role"`
	AttachmentFileID *int64         `gorm:"column:attachment_file_id"`
	Attachment       *File          `gorm:"foreignKey:AttachmentFileID;references:ID"`
	ToolCallID       *string        `gorm:"column:tool_call_id"`
	ToolName         *string        `gorm:"column:tool_name"`
	ToolCallsJSON    *string        `gorm:"column:tool_calls_json"`
	CreatedAt        time.Time      `gorm:"column:created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (Message) TableName() string {
	return "messages"
}
