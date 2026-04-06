package postgres

import (
	"context"
	"errors"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type chatPreferenceRepository struct {
	db *gorm.DB
}

func NewChatPreferenceRepository(db *gorm.DB) domain.ChatPreferenceRepository {
	return &chatPreferenceRepository{db: db}
}

func (r *chatPreferenceRepository) GetSelectedRunner(ctx context.Context, userID int) (string, error) {
	var row model.Chat
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}

		return "", err
	}

	return row.SelectedRunner, nil
}

func (r *chatPreferenceRepository) SetSelectedRunner(ctx context.Context, userID int, runner string) error {
	return r.db.WithContext(ctx).Model(&model.Chat{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{
			"selected_runner": runner,
			"updated_at":      gorm.Expr("NOW()"),
		}).Error
}

func (r *chatPreferenceRepository) GetDefaultRunnerModel(ctx context.Context, userID int, runner string) (string, error) {
	var row model.UserRunnerModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND runner_address = ?", userID, runner).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}

		return "", err
	}

	return row.Model, nil
}

func (r *chatPreferenceRepository) SetDefaultRunnerModel(ctx context.Context, userID int, runner string, modelName string) error {
	row := model.UserRunnerModel{
		UserID:        userID,
		RunnerAddress: runner,
		Model:         modelName,
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "runner_address"}},
		DoUpdates: clause.Assignments(map[string]any{
			"model":      gorm.Expr("EXCLUDED.model"),
			"updated_at": gorm.Expr("NOW()"),
		}),
	}).Create(&row).Error
}
