package usecase

import (
	"context"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
)

func forwardLLMStreamChunks(
	ctx context.Context,
	out chan<- ChatStreamChunk,
	messageID int64,
	in <-chan domain.LLMStreamChunk,
	intoContent *strings.Builder,
) {
	for chunk := range in {
		if chunk.ReasoningContent != "" {
			select {
			case <-ctx.Done():
				return
			case out <- ChatStreamChunk{Kind: StreamChunkKindReasoning, Text: chunk.ReasoningContent, MessageID: messageID}:
			}
		}

		if chunk.Content != "" {
			intoContent.WriteString(chunk.Content)
			select {
			case <-ctx.Done():
				return
			case out <- ChatStreamChunk{Kind: StreamChunkKindText, Text: chunk.Content, MessageID: messageID}:
			}
		}
	}
}
