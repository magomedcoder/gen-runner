package model

import (
	"encoding/json"
	"time"
)

type DocumentRAGChunk struct {
	ID                  int64           `gorm:"column:id;primaryKey;autoIncrement"`
	ChatSessionID       int64           `gorm:"column:chat_session_id"`
	UserID              int             `gorm:"column:user_id"`
	FileID              int64           `gorm:"column:file_id"`
	ChunkIndex          int             `gorm:"column:chunk_index"`
	Text                string          `gorm:"column:text"`
	Metadata            json.RawMessage `gorm:"column:metadata;type:jsonb"`
	ChunkContentSHA256  string          `gorm:"column:chunk_content_sha256"`
	SourceContentSHA256 string          `gorm:"column:source_content_sha256"`
	PipelineVersion     string          `gorm:"column:pipeline_version"`
	EmbeddingModel      string          `gorm:"column:embedding_model"`
	EmbeddingDim        int             `gorm:"column:embedding_dim"`
	Embedding           []byte          `gorm:"column:embedding"`
	CreatedAt           time.Time       `gorm:"column:created_at"`
}

func (DocumentRAGChunk) TableName() string {
	return "document_rag_chunks"
}

type FileRAGIndex struct {
	FileID              int64     `gorm:"column:file_id;primaryKey"`
	Status              string    `gorm:"column:status"`
	LastError           *string   `gorm:"column:last_error"`
	SourceContentSHA256 string    `gorm:"column:source_content_sha256"`
	PipelineVersion     string    `gorm:"column:pipeline_version"`
	EmbeddingModel      string    `gorm:"column:embedding_model"`
	ChunkCount          int       `gorm:"column:chunk_count"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

func (FileRAGIndex) TableName() string {
	return "file_rag_index"
}
