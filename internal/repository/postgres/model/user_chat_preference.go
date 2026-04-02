package model

import "time"

type UserChatPreference struct {
	UserID         int       `gorm:"column:user_id;primaryKey"`
	SelectedRunner string    `gorm:"column:selected_runner"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (UserChatPreference) TableName() string {
	return "user_chat_preferences"
}
