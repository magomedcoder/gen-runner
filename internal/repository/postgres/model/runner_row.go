package model

import "time"

type RunnerRow struct {
	ID            int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Name          string    `gorm:"column:name"`
	Host          string    `gorm:"column:host"`
	Port          int32     `gorm:"column:port"`
	Enabled       bool      `gorm:"column:enabled"`
	SelectedModel string    `gorm:"column:selected_model"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (RunnerRow) TableName() string {
	return "runners"
}
