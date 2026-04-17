package usecase

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
)

const (
	ragChunkTextPreviewMaxRunes    = 360
	ragFullDocumentPreviewMaxRunes = 1800
)

const (
	ragModeFullDocument  = "full_document"
	ragModeVectorRAG     = "vector_rag"
	ragModeVectorRAGDeep = "vector_rag_deep"
)

type ragSourcesWire struct {
	Mode                string         `json:"mode"`
	FileID              int64          `json:"file_id"`
	TopK                int            `json:"top_k,omitempty"`
	NeighborWindow      int            `json:"neighbor_window,omitempty"`
	DeepRAGMapCalls     int            `json:"deep_rag_map_calls,omitempty"`
	DroppedByBudget     int            `json:"dropped_by_budget,omitempty"`
	FullDocumentExcerpt string         `json:"full_document_excerpt,omitempty"`
	Chunks              []ragChunkWire `json:"chunks"`
}

type ragChunkWire struct {
	ChunkIndex   int     `json:"chunk_index"`
	Score        float64 `json:"score"`
	IsNeighbor   bool    `json:"is_neighbor"`
	HeadingPath  string  `json:"heading_path,omitempty"`
	PdfPageStart int     `json:"pdf_page_start,omitempty"`
	PdfPageEnd   int     `json:"pdf_page_end,omitempty"`
	Excerpt      string  `json:"excerpt,omitempty"`
}

func (w *ragSourcesWire) toPayload() *RAGSourcesPayload {
	chunks := make([]RAGChunkPreview, 0, len(w.Chunks))
	for _, c := range w.Chunks {
		chunks = append(chunks, RAGChunkPreview{
			ChunkIndex:   int32(c.ChunkIndex),
			Score:        c.Score,
			IsNeighbor:   c.IsNeighbor,
			HeadingPath:  c.HeadingPath,
			PdfPageStart: int32(c.PdfPageStart),
			PdfPageEnd:   int32(c.PdfPageEnd),
			Excerpt:      c.Excerpt,
		})
	}

	return &RAGSourcesPayload{
		Mode:                w.Mode,
		FileID:              w.FileID,
		TopK:                int32(w.TopK),
		NeighborWindow:      int32(w.NeighborWindow),
		DeepRAGMapCalls:     int32(w.DeepRAGMapCalls),
		DroppedByBudget:     int32(w.DroppedByBudget),
		FullDocumentExcerpt: w.FullDocumentExcerpt,
		Chunks:              chunks,
	}
}

func truncateRAGPreviewText(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if s == "" || maxRunes <= 0 {
		return ""
	}

	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}

	r := []rune(s)
	return string(r[:maxRunes]) + "…"
}

type ragStreamMeta struct {
	Mode        string
	SourcesJSON string
	Sources     *RAGSourcesPayload
	ShortNotice string
}

func (m *ragStreamMeta) asChunk() ChatStreamChunk {
	return ChatStreamChunk{
		Kind:           StreamChunkKindRAGMeta,
		Text:           m.ShortNotice,
		RAGMode:        m.Mode,
		RAGSourcesJSON: m.SourcesJSON,
		RAGSources:     m.Sources,
		MessageID:      0,
	}
}

func buildRAGStreamMetaFullDocument(fileID int64, extractedText string) (*ragStreamMeta, error) {
	ex := truncateRAGPreviewText(extractedText, ragFullDocumentPreviewMaxRunes)
	w := ragSourcesWire{
		Mode:                ragModeFullDocument,
		FileID:              fileID,
		Chunks:              []ragChunkWire{},
		FullDocumentExcerpt: ex,
	}

	js, err := json.Marshal(w)
	if err != nil {
		return nil, err
	}

	src := w.toPayload()

	return &ragStreamMeta{
		Mode:        ragModeFullDocument,
		SourcesJSON: string(js),
		Sources:     src,
		ShortNotice: "Контекст документа: полный текст вложения (без векторного поиска).",
	}, nil
}

func buildRAGStreamMetaVector(
	fileID int64,
	topK int,
	neighborWindow int,
	scored []domain.ScoredDocumentRAGChunk,
	deepMapCalls int,
	deepSummaryUsed bool,
	droppedByBudget int,
) (*ragStreamMeta, error) {
	mode := ragModeVectorRAG
	if deepSummaryUsed {
		mode = ragModeVectorRAGDeep
	}

	chunks := make([]ragChunkWire, 0, len(scored))
	for _, sc := range scored {
		meta := sc.DocumentRAGChunk.Metadata
		cw := ragChunkWire{
			ChunkIndex: sc.DocumentRAGChunk.ChunkIndex,
			Score:      sc.Score,
			IsNeighbor: sc.Score <= ragNeighborOnlyChunkScore/10,
		}

		if hp, ok := meta["heading_path"].(string); ok {
			cw.HeadingPath = strings.TrimSpace(hp)
		}

		if ps, ok := intFromMeta(meta, "pdf_page_start"); ok {
			cw.PdfPageStart = ps
		}

		if pe, ok := intFromMeta(meta, "pdf_page_end"); ok {
			cw.PdfPageEnd = pe
		}

		cw.Excerpt = truncateRAGPreviewText(sc.Text, ragChunkTextPreviewMaxRunes)
		chunks = append(chunks, cw)
	}

	w := ragSourcesWire{
		Mode:            mode,
		FileID:          fileID,
		TopK:            topK,
		NeighborWindow:  neighborWindow,
		Chunks:          chunks,
		DroppedByBudget: droppedByBudget,
	}

	if deepMapCalls > 0 {
		w.DeepRAGMapCalls = deepMapCalls
	}

	js, err := json.Marshal(w)
	if err != nil {
		return nil, err
	}

	src := w.toPayload()

	label := "быстрый векторный RAG (top-K + соседи)"
	if mode == ragModeVectorRAGDeep {
		label = fmt.Sprintf("векторный RAG + сжатие фрагментов (deep), map-вызовов=%d", deepMapCalls)
	}

	notice := fmt.Sprintf("Контекст документа: %s. Фрагментов в промпте: %d.", label, len(chunks))
	if droppedByBudget > 0 {
		notice += fmt.Sprintf(" Отброшено по бюджету: %d.", droppedByBudget)
	}

	return &ragStreamMeta{
		Mode:        mode,
		SourcesJSON: string(js),
		Sources:     src,
		ShortNotice: notice,
	}, nil
}
