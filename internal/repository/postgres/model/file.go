package model

import "time"

type File struct {
	ID                         int64      `gorm:"column:id;primaryKey;autoIncrement"`
	Filename                   string     `gorm:"column:filename"`
	MimeType                   *string    `gorm:"column:mime_type"`
	Size                       int64      `gorm:"column:size"`
	StoragePath                string     `gorm:"column:storage_path"`
	ChatSessionID              *int64     `gorm:"column:chat_session_id"`
	UserID                     *int       `gorm:"column:user_id"`
	ExpiresAt                  *time.Time `gorm:"column:expires_at"`
	Kind                       string     `gorm:"column:kind"`
	CreatedAt                  time.Time  `gorm:"column:created_at"`
	ExtractedText              *string    `gorm:"column:extracted_text"`
	ExtractedTextContentSha256 *string    `gorm:"column:extracted_text_content_sha256"`
}

func (File) TableName() string {
	return "files"
}
