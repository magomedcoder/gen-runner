package usecase

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
)

func TestBuildRAGStreamMetaFullDocument_excerpt(t *testing.T) {
	m, err := buildRAGStreamMetaFullDocument(7, "  hello очень длинный текст для превью  ")
	if err != nil {
		t.Fatal(err)
	}

	var w ragSourcesWire
	if err := json.Unmarshal([]byte(m.SourcesJSON), &w); err != nil {
		t.Fatal(err)
	}

	if w.Mode != ragModeFullDocument || w.FileID != 7 {
		t.Fatalf("wire-структура: %+v", w)
	}

	if !strings.Contains(w.FullDocumentExcerpt, "hello") {
		t.Fatalf("выдержка: %q", w.FullDocumentExcerpt)
	}
}

func TestBuildRAGStreamMetaVector_modes(t *testing.T) {
	scored := []domain.ScoredDocumentRAGChunk{
		{
			DocumentRAGChunk: domain.DocumentRAGChunk{
				ChunkIndex: 1,
				Text:       "alpha beta gamma delta preview body",
				Metadata: map[string]any{
					"heading_path":   "Intro › A",
					"pdf_page_start": float64(2),
					"pdf_page_end":   float64(2),
				},
			},
			Score: 0.88,
		},
	}

	m, err := buildRAGStreamMetaVector(42, 5, 2, scored, 0, false, 1)
	if err != nil {
		t.Fatal(err)
	}

	if m.Mode != ragModeVectorRAG {
		t.Fatalf("режим: %s", m.Mode)
	}

	var w ragSourcesWire
	if err := json.Unmarshal([]byte(m.SourcesJSON), &w); err != nil {
		t.Fatal(err)
	}

	if w.FileID != 42 || w.TopK != 5 || w.NeighborWindow != 2 || len(w.Chunks) != 1 {
		t.Fatalf("wire-структура: %+v", w)
	}

	if w.DroppedByBudget != 1 {
		t.Fatalf("ожидали dropped_by_budget=1, получено %d", w.DroppedByBudget)
	}

	if w.Chunks[0].HeadingPath != "Intro › A" || w.Chunks[0].ChunkIndex != 1 {
		t.Fatalf("чанк: %+v", w.Chunks[0])
	}

	if w.Chunks[0].Excerpt == "" || !strings.Contains(w.Chunks[0].Excerpt, "preview") {
		t.Fatalf("выдержка: %q", w.Chunks[0].Excerpt)
	}

	m2, err := buildRAGStreamMetaVector(1, 3, 0, scored, 2, true, 0)
	if err != nil {
		t.Fatal(err)
	}

	if m2.Mode != ragModeVectorRAGDeep {
		t.Fatalf("режим deep: %s", m2.Mode)
	}
}
