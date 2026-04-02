package postgres

import (
	"context"
	"os"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type fileRepository struct {
	db *gorm.DB
}

func NewFileRepository(db *gorm.DB) domain.FileRepository {
	return &fileRepository{db: db}
}

func (r *fileRepository) Create(ctx context.Context, file *domain.File) error {
	kind := strings.TrimSpace(file.Kind)
	row := model.File{
		Filename:      file.Filename,
		MimeType:      nullStringPtr(file.MimeType),
		Size:          file.Size,
		StoragePath:   file.StoragePath,
		CreatedAt:     file.CreatedAt,
		ChatSessionID: file.ChatSessionID,
		UserID:        file.UserID,
		ExpiresAt:     file.ExpiresAt,
		Kind:          kind,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	file.Id = row.ID
	return nil
}

func (r *fileRepository) UpdateStoragePath(ctx context.Context, id int64, storagePath string) error {
	return r.db.WithContext(ctx).Model(&model.File{}).
		Where("id = ?", id).
		Update("storage_path", storagePath).Error
}

func (r *fileRepository) GetById(ctx context.Context, id int64) (*domain.File, error) {
	var row model.File
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return fileToDomain(&row), nil
}

func (r *fileRepository) DeleteExpired(ctx context.Context) (deleted int64, err error) {
	rows, err := r.db.WithContext(ctx).Raw(`
		DELETE FROM files
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
		RETURNING storage_path
	`).Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var n int64
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return n, err
		}
		if p != "" && p != "." {
			_ = os.Remove(p)
		}
		n++
	}
	return n, rows.Err()
}

func (r *fileRepository) CountSessionTTLArtifacts(ctx context.Context, sessionID int64, userID int) (count int32, totalSize int64, err error) {
	type agg struct {
		Cnt  int32 `gorm:"column:cnt"`
		Size int64 `gorm:"column:total_size"`
	}
	var a agg
	err = r.db.WithContext(ctx).Model(&model.File{}).
		Select("COUNT(*)::int AS cnt", "COALESCE(SUM(size), 0)::bigint AS total_size").
		Where("chat_session_id = ? AND user_id = ? AND kind = ?", sessionID, userID, "artifact").
		Where("expires_at IS NOT NULL AND expires_at > NOW()").
		Scan(&a).Error
	return a.Cnt, a.Size, err
}

func fileToDomain(m *model.File) *domain.File {
	f := &domain.File{
		Id:            m.ID,
		Filename:      m.Filename,
		Size:          m.Size,
		StoragePath:   m.StoragePath,
		CreatedAt:     m.CreatedAt,
		ChatSessionID: m.ChatSessionID,
		UserID:        m.UserID,
		ExpiresAt:     m.ExpiresAt,
		Kind:          m.Kind,
	}
	if m.MimeType != nil {
		f.MimeType = *m.MimeType
	}
	return f
}

func nullStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
