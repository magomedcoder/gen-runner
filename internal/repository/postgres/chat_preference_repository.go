package postgres

import (
	"context"
	"errors"
	"strings"

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
	var row model.UserChatPreference
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(row.SelectedRunner), nil
}

func (r *chatPreferenceRepository) SetSelectedRunner(ctx context.Context, userID int, runner string) error {
	row := model.UserChatPreference{
		UserID:         userID,
		SelectedRunner: strings.TrimSpace(runner),
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"selected_runner": strings.TrimSpace(runner),
			"updated_at":      gorm.Expr("NOW()"),
		}),
	}).Create(&row).Error
}

func (r *chatPreferenceRepository) GetDefaultRunnerModel(ctx context.Context, userID int, runner string) (string, error) {
	var row model.UserRunnerModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND runner_address = ?", userID, strings.TrimSpace(runner)).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(row.Model), nil
}

func (r *chatPreferenceRepository) SetDefaultRunnerModel(ctx context.Context, userID int, runner string, modelName string) error {
	runner = strings.TrimSpace(runner)
	modelName = strings.TrimSpace(modelName)
	if runner == "" {
		return nil
	}
	if modelName == "" {
		return r.db.WithContext(ctx).
			Where("user_id = ? AND runner_address = ?", userID, runner).
			Delete(&model.UserRunnerModel{}).Error
	}
	row := model.UserRunnerModel{
		UserID:        userID,
		RunnerAddress: runner,
		Model:         modelName,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "runner_address"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"model":      modelName,
			"updated_at": gorm.Expr("NOW()"),
		}),
	}).Create(&row).Error
}
