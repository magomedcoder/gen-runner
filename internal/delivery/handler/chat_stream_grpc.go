package handler

import (
	"strings"
	"time"

	"github.com/magomedcoder/gen/api/pb/chatpb"
	"github.com/magomedcoder/gen/internal/usecase"
)

func streamChunkKindToPB(kind usecase.StreamChunkKind) chatpb.StreamChunkKind {
	switch kind {
	case usecase.StreamChunkKindToolStatus:
		return chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TOOL_STATUS
	case usecase.StreamChunkKindNotice:
		return chatpb.StreamChunkKind_STREAM_CHUNK_KIND_NOTICE
	case usecase.StreamChunkKindReasoning:
		return chatpb.StreamChunkKind_STREAM_CHUNK_KIND_REASONING
	case usecase.StreamChunkKindRAGMeta:
		return chatpb.StreamChunkKind_STREAM_CHUNK_KIND_RAG_META
	default:
		return chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT
	}
}

func streamChunkRole(kind usecase.StreamChunkKind) string {
	if kind == usecase.StreamChunkKindNotice || kind == usecase.StreamChunkKindRAGMeta {
		return "system"
	}
	return "assistant"
}

func ragSourcesPayloadToPB(p *usecase.RAGSourcesPayload) *chatpb.RagSourcesPayload {
	if p == nil {
		return nil
	}

	out := &chatpb.RagSourcesPayload{
		Mode:                p.Mode,
		FileId:              p.FileID,
		TopK:                p.TopK,
		NeighborWindow:      p.NeighborWindow,
		DeepRagMapCalls:     p.DeepRAGMapCalls,
		DroppedByBudget:     p.DroppedByBudget,
		FullDocumentExcerpt: p.FullDocumentExcerpt,
	}

	for _, c := range p.Chunks {
		out.Chunks = append(out.Chunks, &chatpb.RagChunkPreview{
			ChunkIndex:   c.ChunkIndex,
			Score:        c.Score,
			IsNeighbor:   c.IsNeighbor,
			HeadingPath:  c.HeadingPath,
			PdfPageStart: c.PdfPageStart,
			PdfPageEnd:   c.PdfPageEnd,
			Excerpt:      c.Excerpt,
		})
	}

	return out
}

func streamSendLoop(responseChan <-chan usecase.ChatStreamChunk, send func(*chatpb.ChatResponse) error) error {
	createdAt := time.Now().Unix()
	var lastMsgID int64
	var accText, accReasoning strings.Builder

	for chunk := range responseChan {
		if (chunk.Kind == usecase.StreamChunkKindText || chunk.Kind == usecase.StreamChunkKindReasoning) && chunk.MessageID != 0 {
			lastMsgID = chunk.MessageID
		}

		switch chunk.Kind {
		case usecase.StreamChunkKindText:
			accText.WriteString(chunk.Text)
		case usecase.StreamChunkKindReasoning:
			accReasoning.WriteString(chunk.Text)
		}

		respID := chunk.MessageID
		if respID == 0 {
			respID = lastMsgID
		}

		resp := &chatpb.ChatResponse{
			Id:        respID,
			Content:   chunk.Text,
			Role:      streamChunkRole(chunk.Kind),
			CreatedAt: createdAt,
			Done:      false,
			ChunkKind: streamChunkKindToPB(chunk.Kind),
		}

		if chunk.ToolName != "" {
			tn := chunk.ToolName
			resp.ToolName = &tn
		}

		if chunk.RAGMode != "" {
			rm := chunk.RAGMode
			resp.RagMode = &rm
		}

		if chunk.RAGSourcesJSON != "" {
			rj := chunk.RAGSourcesJSON
			resp.RagSourcesJson = &rj
		}

		if chunk.RAGSources != nil {
			resp.RagSources = ragSourcesPayloadToPB(chunk.RAGSources)
		}

		if err := send(resp); err != nil {
			return err
		}
	}

	final := &chatpb.AssistantStreamFinal{
		AssistantMessageId: lastMsgID,
		Text:               accText.String(),
		Reasoning:          accReasoning.String(),
	}

	return send(&chatpb.ChatResponse{
		Id:             lastMsgID,
		Content:        "",
		Role:           "assistant",
		CreatedAt:      createdAt,
		Done:           true,
		ChunkKind:      chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT,
		AssistantFinal: final,
	})
}
