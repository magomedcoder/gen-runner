package postgres

import (
	"context"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type userSessionRepository struct {
	db *gorm.DB
}

func NewUserSessionRepository(db *gorm.DB) domain.TokenRepository {
	return &userSessionRepository{db: db}
}

func (u *userSessionRepository) Create(ctx context.Context, token *domain.Token) error {
	row := model.UserSession{
		UserID:    token.UserId,
		Token:     token.Token,
		Type:      string(token.Type),
		ExpiresAt: token.ExpiresAt,
		CreatedAt: token.CreatedAt,
	}
	if err := u.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	token.Id = row.ID
	return nil
}

func (u *userSessionRepository) GetByToken(ctx context.Context, token string) (*domain.Token, error) {
	var row model.UserSession
	err := u.db.WithContext(ctx).
		Where("token = ?", token).
		First(&row).Error
	if err != nil {
		return nil, handleNotFound(err, "токен не найден")
	}
	return &domain.Token{
		Id:        row.ID,
		UserId:    row.UserID,
		Token:     row.Token,
		Type:      domain.TokenType(row.Type),
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
		DeletedAt: gormDeletedAtToPtr(row.DeletedAt),
	}, nil
}

func (u *userSessionRepository) DeleteByToken(ctx context.Context, token string) error {
	return u.db.WithContext(ctx).
		Where("token = ?", token).
		Delete(&model.UserSession{}).Error
}

func (u *userSessionRepository) DeleteByUserId(ctx context.Context, userID int, tokenType domain.TokenType) error {
	return u.db.WithContext(ctx).
		Where("user_id = ? AND type = ?", userID, string(tokenType)).
		Delete(&model.UserSession{}).Error
}
