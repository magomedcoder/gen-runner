package usecase

type StreamChunkKind int

const (
	StreamChunkKindText StreamChunkKind = iota
	StreamChunkKindToolStatus
	StreamChunkKindNotice
)

type ChatStreamChunk struct {
	Kind      StreamChunkKind
	Text      string
	ToolName  string
	MessageID int64
}
