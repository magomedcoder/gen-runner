package postgres

import (
	"context"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type editorHistoryRepository struct {
	db *gorm.DB
}

func NewEditorHistoryRepository(db *gorm.DB) domain.EditorHistoryRepository {
	return &editorHistoryRepository{db: db}
}

func (r *editorHistoryRepository) Save(ctx context.Context, userID int, runner string, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	row := model.EditorTextHistory{
		UserID: userID,
		Runner: strings.TrimSpace(runner),
		Text:   text,
	}
	return r.db.WithContext(ctx).Omit("ID", "CreatedAt").Create(&row).Error
}
