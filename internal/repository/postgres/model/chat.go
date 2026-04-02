package model

import (
	"time"

	"gorm.io/gorm"
)

type Chat struct {
	ID        int64          `gorm:"column:id;primaryKey;autoIncrement"`
	UserID    int            `gorm:"column:user_id"`
	Title     string         `gorm:"column:title"`
	Model     string         `gorm:"column:model"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (Chat) TableName() string {
	return "chats"
}
