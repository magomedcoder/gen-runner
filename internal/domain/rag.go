package domain

import "time"

const RAGPipelineVersion = "v1"

const (
	FileRAGIndexStatusPending  = "pending"
	FileRAGIndexStatusIndexing = "indexing"
	FileRAGIndexStatusReady    = "ready"
	FileRAGIndexStatusFailed   = "failed"
)

type FileRAGIndex struct {
	FileID              int64
	Status              string
	LastError           string
	SourceContentSHA256 string
	PipelineVersion     string
	EmbeddingModel      string
	ChunkCount          int
	UpdatedAt           time.Time
}

type DocumentRAGChunk struct {
	ID                  int64
	ChatSessionID       int64
	UserID              int
	FileID              int64
	ChunkIndex          int
	Text                string
	Metadata            map[string]any
	ChunkContentSHA256  string
	SourceContentSHA256 string
	PipelineVersion     string
	EmbeddingModel      string
	EmbeddingDim        int
	Embedding           []float32
	CreatedAt           time.Time
}

type ScoredDocumentRAGChunk struct {
	DocumentRAGChunk
	Score float64
}
