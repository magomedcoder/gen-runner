package postgres

import (
	"context"
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
	q := r.db.WithContext(ctx).Where("id = ?", id).Omit("ExtractedText", "ExtractedTextContentSha256")
	if err := q.First(&row).Error; err != nil {
		return nil, err
	}
	return fileToDomain(&row), nil
}

func (r *fileRepository) GetByIdWithExtractedCache(ctx context.Context, id int64) (*domain.File, error) {
	var row model.File
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return fileToDomain(&row), nil
}

func (r *fileRepository) ListByIds(ctx context.Context, ids []int64) ([]*domain.File, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var rows []model.File
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]*domain.File, 0, len(rows))
	for i := range rows {
		out = append(out, fileToDomain(&rows[i]))
	}

	return out, nil
}

func (r *fileRepository) SaveExtractedTextCache(ctx context.Context, fileID int64, contentSHA256Hex string, extractedText string) error {
	return r.db.WithContext(ctx).Model(&model.File{}).
		Where("id = ?", fileID).
		Updates(map[string]any{
			"extracted_text":                extractedText,
			"extracted_text_content_sha256": contentSHA256Hex,
		}).Error
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
	if m.ExtractedText != nil {
		f.ExtractedText = *m.ExtractedText
	}

	if m.ExtractedTextContentSha256 != nil {
		f.ExtractedTextContentSha256 = *m.ExtractedTextContentSha256
	}

	return f
}

func nullStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
