package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID            int            `gorm:"column:id;primaryKey;autoIncrement"`
	Username      string         `gorm:"column:username"`
	Password      string         `gorm:"column:password"`
	Name          string         `gorm:"column:name"`
	Surname       string         `gorm:"column:surname"`
	Role          int16          `gorm:"column:role"`
	CreatedAt     time.Time      `gorm:"column:created_at"`
	UpdatedAt     time.Time      `gorm:"column:updated_at"`
	LastVisitedAt *time.Time     `gorm:"column:last_visited_at"`
	DeletedAt     gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (User) TableName() string {
	return "users"
}
