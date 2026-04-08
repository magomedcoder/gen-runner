package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type runnerRepository struct {
	db *gorm.DB
}

func NewRunnerRepository(db *gorm.DB) domain.RunnerRepository {
	return &runnerRepository{db: db}
}

func rowToRunner(m *model.RunnerRow) domain.Runner {
	return domain.Runner{
		ID:            m.ID,
		Name:          m.Name,
		Host:          m.Host,
		Port:          m.Port,
		Enabled:       m.Enabled,
		SelectedModel: strings.TrimSpace(m.SelectedModel),
	}
}

func (r *runnerRepository) List(ctx context.Context) ([]domain.Runner, error) {
	var rows []model.RunnerRow
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Runner, 0, len(rows))
	for i := range rows {
		out = append(out, rowToRunner(&rows[i]))
	}
	return out, nil
}

func (r *runnerRepository) GetByID(ctx context.Context, id int64) (*domain.Runner, error) {
	var row model.RunnerRow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	ru := rowToRunner(&row)
	return &ru, nil
}

func (r *runnerRepository) FirstEnabled(ctx context.Context) (*domain.Runner, error) {
	var row model.RunnerRow
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("id ASC").First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	ru := rowToRunner(&row)
	return &ru, nil
}

func (r *runnerRepository) Create(ctx context.Context, name, host string, port int32, enabled bool, selectedModel string) (*domain.Runner, error) {
	row := model.RunnerRow{
		Name:          strings.TrimSpace(name),
		Host:          strings.TrimSpace(host),
		Port:          port,
		Enabled:       enabled,
		SelectedModel: strings.TrimSpace(selectedModel),
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	ru := rowToRunner(&row)
	return &ru, nil
}

func (r *runnerRepository) Update(ctx context.Context, id int64, name, host string, port int32, enabled bool, selectedModel string) (*domain.Runner, error) {
	var row model.RunnerRow
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	row.Name = strings.TrimSpace(name)
	row.Host = strings.TrimSpace(host)
	row.Port = port
	row.Enabled = enabled
	row.SelectedModel = strings.TrimSpace(selectedModel)
	row.UpdatedAt = time.Now()

	if err := r.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	ru := rowToRunner(&row)
	return &ru, nil
}

func (r *runnerRepository) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	res := r.db.WithContext(ctx).Model(&model.RunnerRow{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"enabled":    enabled,
			"updated_at": gorm.Expr("NOW()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *runnerRepository) Delete(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.RunnerRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *runnerRepository) FindIDByListenAddress(ctx context.Context, listenAddr string) (int64, bool, error) {
	listenAddr = strings.TrimSpace(listenAddr)
	if listenAddr == "" {
		return 0, false, nil
	}
	list, err := r.List(ctx)
	if err != nil {
		return 0, false, err
	}
	for i := range list {
		if domain.RunnerListenAddress(list[i].Host, list[i].Port) == listenAddr {
			return list[i].ID, true, nil
		}
	}
	return 0, false, nil
}
