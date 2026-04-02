package postgres

import "gorm.io/gorm"

func scopeMessageSession(sessionID int64) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("session_id = ?", sessionID)
	}
}

func scopeChatUser(userID int) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("user_id = ?", userID)
	}
}
