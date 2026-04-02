package model

import "time"

type UserRunnerModel struct {
	UserID        int       `gorm:"column:user_id;primaryKey"`
	RunnerAddress string    `gorm:"column:runner_address;primaryKey"`
	Model         string    `gorm:"column:model"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (UserRunnerModel) TableName() string {
	return "user_runner_models"
}
