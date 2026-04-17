package usecase

type StreamChunkKind int

const (
	StreamChunkKindText StreamChunkKind = iota
	StreamChunkKindToolStatus
	StreamChunkKindNotice
	StreamChunkKindReasoning
	StreamChunkKindRAGMeta
)

type RAGChunkPreview struct {
	ChunkIndex   int32
	Score        float64
	IsNeighbor   bool
	HeadingPath  string
	PdfPageStart int32
	PdfPageEnd   int32
	Excerpt      string
}

type RAGSourcesPayload struct {
	Mode                string
	FileID              int64
	TopK                int32
	NeighborWindow      int32
	DeepRAGMapCalls     int32
	DroppedByBudget     int32
	FullDocumentExcerpt string
	Chunks              []RAGChunkPreview
}

type ChatStreamChunk struct {
	Kind           StreamChunkKind
	Text           string
	ToolName       string
	MessageID      int64
	RAGMode        string
	RAGSourcesJSON string
	RAGSources     *RAGSourcesPayload
}
