package postgres

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres/model"
	"gorm.io/gorm"
)

type documentRAGRepository struct {
	db *gorm.DB
}

func NewDocumentRAGRepository(db *gorm.DB) domain.DocumentRAGRepository {
	return &documentRAGRepository{db: db}
}

func float32SliceToBytes(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}

	buf := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}

	return buf
}

func bytesToFloat32Slice(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("embedding: некратная длина bytea")
	}

	n := len(b) / 4
	out := make([]float32, n)
	for i := range n {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}

	return out, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, na, nb float64
	for i := range a {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		na += af * af
		nb += bf * bf
	}

	den := math.Sqrt(na) * math.Sqrt(nb)
	if den == 0 {
		return 0
	}

	return dot / den
}

func (r *documentRAGRepository) GetFileIndex(ctx context.Context, fileID int64) (*domain.FileRAGIndex, error) {
	var row model.FileRAGIndex
	err := r.db.WithContext(ctx).Where("file_id = ?", fileID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return fileRAGIndexToDomain(&row), nil
}

func (r *documentRAGRepository) SaveFileRAGIndex(ctx context.Context, row *domain.FileRAGIndex) error {
	if row == nil {
		return fmt.Errorf("file rag index: nil")
	}

	m := fileRAGIndexFromDomain(row)

	return r.db.WithContext(ctx).Save(&m).Error
}

func (r *documentRAGRepository) MarkFileRAGIndexFailed(ctx context.Context, fileID int64, errMsg string) error {
	msg := strings.TrimSpace(errMsg)
	if msg == "" {
		msg = "неизвестная ошибка индексации"
	}

	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.FileRAGIndex{}).Where("file_id = ?", fileID).Updates(map[string]any{
		"status":     domain.FileRAGIndexStatusFailed,
		"last_error": msg,
		"updated_at": now,
	})

	if res.Error != nil {
		return res.Error
	}

	if res.RowsAffected > 0 {
		return nil
	}

	last := msg
	return r.db.WithContext(ctx).Create(&model.FileRAGIndex{
		FileID:              fileID,
		Status:              domain.FileRAGIndexStatusFailed,
		LastError:           &last,
		SourceContentSHA256: "",
		PipelineVersion:     "",
		EmbeddingModel:      "",
		ChunkCount:          0,
		UpdatedAt:           now,
	}).Error
}

func (r *documentRAGRepository) ReplaceFileChunks(
	ctx context.Context,
	sessionID int64,
	userID int,
	fileID int64,
	pipelineVersion, embeddingModel, sourceSHA256 string,
	chunks []domain.DocumentRAGChunk,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Where("file_id = ? AND embedding_model = ? AND pipeline_version = ?", fileID, embeddingModel, pipelineVersion)
		if err := q.Delete(&model.DocumentRAGChunk{}).Error; err != nil {
			return err
		}

		rows := make([]model.DocumentRAGChunk, 0, len(chunks))
		for i := range chunks {
			c := &chunks[i]
			meta := c.Metadata
			if meta == nil {
				meta = map[string]any{}
			}

			metaJSON, err := json.Marshal(meta)
			if err != nil {
				return err
			}

			if len(metaJSON) == 0 {
				metaJSON = []byte("{}")
			}

			emb := float32SliceToBytes(c.Embedding)
			if len(c.Embedding) > 0 && len(emb) == 0 {
				return fmt.Errorf("embedding: пустой bytea")
			}

			rows = append(rows, model.DocumentRAGChunk{
				ChatSessionID:       sessionID,
				UserID:              userID,
				FileID:              fileID,
				ChunkIndex:          c.ChunkIndex,
				Text:                c.Text,
				Metadata:            metaJSON,
				ChunkContentSHA256:  c.ChunkContentSHA256,
				SourceContentSHA256: sourceSHA256,
				PipelineVersion:     pipelineVersion,
				EmbeddingModel:      embeddingModel,
				EmbeddingDim:        len(c.Embedding),
				Embedding:           emb,
				CreatedAt:           time.Now(),
			})
		}

		if len(rows) > 0 {
			if err := tx.CreateInBatches(&rows, 128).Error; err != nil {
				return err
			}
		}

		idx := model.FileRAGIndex{
			FileID:              fileID,
			Status:              domain.FileRAGIndexStatusReady,
			LastError:           nil,
			SourceContentSHA256: sourceSHA256,
			PipelineVersion:     pipelineVersion,
			EmbeddingModel:      embeddingModel,
			ChunkCount:          len(rows),
			UpdatedAt:           time.Now(),
		}
		return tx.Save(&idx).Error
	})
}

func (r *documentRAGRepository) DeleteIndexForFile(ctx context.Context, fileID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("file_id = ?", fileID).Delete(&model.DocumentRAGChunk{}).Error; err != nil {
			return err
		}

		return tx.Where("file_id = ?", fileID).Delete(&model.FileRAGIndex{}).Error
	})
}

func (r *documentRAGRepository) SearchSessionTopK(
	ctx context.Context,
	sessionID int64,
	userID int,
	embeddingModel string,
	queryEmbedding []float32,
	topK int,
	fileID *int64,
) ([]domain.ScoredDocumentRAGChunk, error) {
	if topK <= 0 {
		return nil, nil
	}

	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("пустой вектор запроса")
	}

	q := r.db.WithContext(ctx).Where("chat_session_id = ? AND user_id = ? AND embedding_model = ?", sessionID, userID, embeddingModel)
	if fileID != nil && *fileID > 0 {
		q = q.Where("file_id = ?", *fileID)
	}

	var rows []model.DocumentRAGChunk
	err := q.Find(&rows).Error
	if err != nil {
		return nil, err
	}

	type scored struct {
		row   model.DocumentRAGChunk
		score float64
	}

	var buf []scored
	for _, row := range rows {
		vec, err := bytesToFloat32Slice(row.Embedding)
		if err != nil || len(vec) == 0 {
			continue
		}

		buf = append(buf, scored{row: row, score: cosineSimilarity(queryEmbedding, vec)})
	}

	sort.Slice(buf, func(i, j int) bool {
		return buf[i].score > buf[j].score
	})

	if len(buf) > topK {
		buf = buf[:topK]
	}

	out := make([]domain.ScoredDocumentRAGChunk, 0, len(buf))
	for _, s := range buf {
		dc, err := documentRAGChunkToDomain(&s.row)
		if err != nil {
			continue
		}

		out = append(out, domain.ScoredDocumentRAGChunk{DocumentRAGChunk: dc, Score: s.score})
	}

	return out, nil
}

func fileRAGIndexToDomain(m *model.FileRAGIndex) *domain.FileRAGIndex {
	d := &domain.FileRAGIndex{
		FileID:              m.FileID,
		Status:              m.Status,
		SourceContentSHA256: m.SourceContentSHA256,
		PipelineVersion:     m.PipelineVersion,
		EmbeddingModel:      m.EmbeddingModel,
		ChunkCount:          m.ChunkCount,
		UpdatedAt:           m.UpdatedAt,
	}

	if m.LastError != nil {
		d.LastError = *m.LastError
	}

	return d
}

func fileRAGIndexFromDomain(d *domain.FileRAGIndex) model.FileRAGIndex {
	var lastErr *string
	if msg := strings.TrimSpace(d.LastError); msg != "" {
		lastErr = new(string)
		*lastErr = msg
	}

	return model.FileRAGIndex{
		FileID:              d.FileID,
		Status:              d.Status,
		LastError:           lastErr,
		SourceContentSHA256: d.SourceContentSHA256,
		PipelineVersion:     d.PipelineVersion,
		EmbeddingModel:      d.EmbeddingModel,
		ChunkCount:          d.ChunkCount,
		UpdatedAt:           d.UpdatedAt,
	}
}

func documentRAGChunkToDomain(m *model.DocumentRAGChunk) (domain.DocumentRAGChunk, error) {
	var meta map[string]any
	if len(m.Metadata) > 0 {
		if err := json.Unmarshal(m.Metadata, &meta); err != nil {
			return domain.DocumentRAGChunk{}, err
		}
	}

	if meta == nil {
		meta = map[string]any{}
	}

	vec, err := bytesToFloat32Slice(m.Embedding)
	if err != nil {
		return domain.DocumentRAGChunk{}, err
	}

	return domain.DocumentRAGChunk{
		ID:                  m.ID,
		ChatSessionID:       m.ChatSessionID,
		UserID:              m.UserID,
		FileID:              m.FileID,
		ChunkIndex:          m.ChunkIndex,
		Text:                m.Text,
		Metadata:            meta,
		ChunkContentSHA256:  m.ChunkContentSHA256,
		SourceContentSHA256: m.SourceContentSHA256,
		PipelineVersion:     m.PipelineVersion,
		EmbeddingModel:      m.EmbeddingModel,
		EmbeddingDim:        m.EmbeddingDim,
		Embedding:           vec,
		CreatedAt:           m.CreatedAt,
	}, nil
}
