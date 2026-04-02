package model

import (
	"time"

	"gorm.io/gorm"
)

type UserSession struct {
	ID        int            `gorm:"column:id;primaryKey;autoIncrement"`
	UserID    int            `gorm:"column:user_id"`
	Token     string         `gorm:"column:token"`
	Type      string         `gorm:"column:type"`
	ExpiresAt time.Time      `gorm:"column:expires_at"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (UserSession) TableName() string {
	return "user_sessions"
}
