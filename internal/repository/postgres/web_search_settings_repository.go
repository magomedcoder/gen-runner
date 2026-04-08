package postgres

import (
	"context"
	"errors"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type webSearchSettingsRepository struct {
	db *gorm.DB
}

func NewWebSearchSettingsRepository(db *gorm.DB) domain.WebSearchSettingsRepository {
	return &webSearchSettingsRepository{db: db}
}

func rowToDomain(m *model.WebSearchSettings) *domain.WebSearchSettings {
	if m == nil {
		return nil
	}

	return &domain.WebSearchSettings{
		Enabled:              m.Enabled,
		MaxResults:           m.MaxResults,
		BraveAPIKey:          m.BraveAPIKey,
		GoogleAPIKey:         m.GoogleAPIKey,
		GoogleSearchEngineID: m.GoogleSearchEngineID,
		YandexUser:           m.YandexUser,
		YandexKey:            m.YandexKey,
	}
}

func (r *webSearchSettingsRepository) Get(ctx context.Context) (*domain.WebSearchSettings, error) {
	var m model.WebSearchSettings
	err := r.db.WithContext(ctx).Where("id = ?", 1).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &domain.WebSearchSettings{MaxResults: 20}, nil
		}

		return nil, err
	}

	return rowToDomain(&m), nil
}

func (r *webSearchSettingsRepository) Upsert(ctx context.Context, s *domain.WebSearchSettings) error {
	if s == nil {
		return nil
	}

	m := model.WebSearchSettings{
		ID:                   1,
		Enabled:              s.Enabled,
		MaxResults:           s.MaxResults,
		BraveAPIKey:          s.BraveAPIKey,
		GoogleAPIKey:         s.GoogleAPIKey,
		GoogleSearchEngineID: s.GoogleSearchEngineID,
		YandexUser:           s.YandexUser,
		YandexKey:            s.YandexKey,
	}

	return r.db.WithContext(ctx).Save(&m).Error
}
