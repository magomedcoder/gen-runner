package postgres

import (
	"context"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) domain.UserRepository {
	return &userRepository{db: db}
}

func (u *userRepository) Create(ctx context.Context, user *domain.User) error {
	row := model.User{
		Username:  user.Username,
		Password:  user.Password,
		Name:      user.Name,
		Surname:   user.Surname,
		Role:      int16(user.Role),
		CreatedAt: user.CreatedAt,
	}
	if err := u.db.WithContext(ctx).Omit("UpdatedAt", "LastVisitedAt").Create(&row).Error; err != nil {
		return err
	}
	user.Id = row.ID
	return nil
}

func (u *userRepository) UpdateLastVisitedAt(ctx context.Context, userID int) error {
	return u.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", userID).
		Update("last_visited_at", gorm.Expr("NOW()")).Error
}

func (u *userRepository) GetById(ctx context.Context, id int) (*domain.User, error) {
	var row model.User
	err := u.db.WithContext(ctx).
		Where("id = ?", id).
		First(&row).Error
	if err != nil {
		return nil, handleNotFound(err, "пользователь не найден")
	}
	return userToDomain(&row), nil
}

func (u *userRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var row model.User
	err := u.db.WithContext(ctx).
		Where("username = ?", username).
		First(&row).Error
	if err != nil {
		return nil, handleNotFound(err, "пользователь не найден")
	}
	return userToDomain(&row), nil
}

func (u *userRepository) Update(ctx context.Context, user *domain.User) error {
	updates := map[string]interface{}{
		"username": user.Username,
		"name":     user.Name,
		"surname":  user.Surname,
		"role":     int16(user.Role),
	}
	if user.Password != "" {
		updates["password"] = user.Password
	}
	return u.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", user.Id).
		Updates(updates).Error
}

func (u *userRepository) List(ctx context.Context, page, pageSize int32) ([]*domain.User, int32, error) {
	_, pageSize, offset := normalizePagination(page, pageSize)

	var total int64
	if err := u.db.WithContext(ctx).Model(&model.User{}).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []model.User
	if err := u.db.WithContext(ctx).
		Order("id DESC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	out := make([]*domain.User, 0, len(rows))
	for i := range rows {
		out = append(out, userToDomain(&rows[i]))
	}
	return out, int32(total), nil
}

func userToDomain(m *model.User) *domain.User {
	return &domain.User{
		Id:            m.ID,
		Username:      m.Username,
		Password:      m.Password,
		Name:          m.Name,
		Surname:       m.Surname,
		Role:          domain.UserRole(m.Role),
		CreatedAt:     m.CreatedAt,
		LastVisitedAt: m.LastVisitedAt,
		DeletedAt:     gormDeletedAtToPtr(m.DeletedAt),
	}
}
