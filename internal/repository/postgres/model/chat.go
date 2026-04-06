package model

import (
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

type Chat struct {
	ID             int64          `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         int            `gorm:"column:user_id"`
	Title          string         `gorm:"column:title"`
	Model          string         `gorm:"column:model"`
	SelectedRunner string         `gorm:"column:selected_runner"`
	SystemPrompt   string         `gorm:"column:system_prompt"`
	StopSequences  pq.StringArray `gorm:"column:stop_sequences;type:text[]"`
	TimeoutSeconds int32          `gorm:"column:timeout_seconds"`
	Temperature    *float32       `gorm:"column:temperature"`
	TopK           *int32         `gorm:"column:top_k"`
	TopP           *float32       `gorm:"column:top_p"`
	JSONMode       bool           `gorm:"column:json_mode"`
	JSONSchema     string         `gorm:"column:json_schema"`
	ToolsJSON      string         `gorm:"column:tools_json"`
	Profile        string         `gorm:"column:profile"`
	CreatedAt      time.Time      `gorm:"column:created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (Chat) TableName() string {
	return "chats"
}
