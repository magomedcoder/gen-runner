package model

import (
	"time"

	"github.com/lib/pq"
)

type ChatSetting struct {
	SessionID      int64          `gorm:"column:session_id;primaryKey"`
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
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
}

func (ChatSetting) TableName() string {
	return "chat_settings"
}
