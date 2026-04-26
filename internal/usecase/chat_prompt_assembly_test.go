package usecase

import (
	"strings"
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
)

func TestAssemblePromptMessages_keepsInstructionSeparateFromDocumentContext(t *testing.T) {
	sessionID := int64(11)
	systemPolicy := domain.NewMessage(sessionID, "system policy", domain.MessageRoleSystem)
	history := []*domain.Message{
		domain.NewMessage(sessionID, "previous assistant", domain.MessageRoleAssistant),
	}

	userInstruction := domain.NewMessage(sessionID, "answer in 3 bullets", domain.MessageRoleUser)
	blocks := []documentContextBlock{
		{Title: "Файл: notes.txt", Body: "```txt\nfacts\n```"},
	}

	out := assemblePromptMessages(sessionID, systemPolicy, history, userInstruction, blocks)
	if len(out) != 4 {
		t.Fatalf("unexpected message count: %d", len(out))
	}

	if out[0].Role != domain.MessageRoleSystem || out[0].Content != "system policy" {
		t.Fatalf("first message must be system policy, got role=%s content=%q", out[0].Role, out[0].Content)
	}

	if out[1].Role != domain.MessageRoleAssistant {
		t.Fatalf("second message must be history assistant, got role=%s", out[1].Role)
	}

	if out[2].Role != domain.MessageRoleSystem {
		t.Fatalf("third message must be document context system message, got role=%s", out[2].Role)
	}

	if !strings.Contains(out[2].Content, documentContextHierarchyInstruction) {
		t.Fatalf("document context block must include hierarchy instruction, got %q", out[2].Content)
	}

	if out[3].Role != domain.MessageRoleUser || out[3].Content != "answer in 3 bullets" {
		t.Fatalf("last message must be raw user instruction, got role=%s content=%q", out[3].Role, out[3].Content)
	}
}

func TestAssemblePromptMessages_withoutDocumentContext(t *testing.T) {
	sessionID := int64(12)
	systemPolicy := domain.NewMessage(sessionID, "sys", domain.MessageRoleSystem)
	history := []*domain.Message{
		domain.NewMessage(sessionID, "prev user", domain.MessageRoleUser),
	}
	userInstruction := domain.NewMessage(sessionID, "latest user request", domain.MessageRoleUser)

	out := assemblePromptMessages(sessionID, systemPolicy, history, userInstruction, nil)
	if len(out) != 3 {
		t.Fatalf("unexpected message count: %d", len(out))
	}

	if out[0] != systemPolicy {
		t.Fatal("system policy must stay first")
	}

	if out[1] != history[0] {
		t.Fatal("history message must be preserved")
	}

	if out[2] != userInstruction {
		t.Fatal("user instruction must stay last")
	}
}

func TestFormatDocumentContextBlock(t *testing.T) {
	got := formatDocumentContextBlock("Файл: a.txt", "```txt\nbody\n```")
	if !strings.Contains(got, "### Файл: a.txt") {
		t.Fatalf("missing heading: %q", got)
	}

	if !strings.Contains(got, "```txt\nbody\n```") {
		t.Fatalf("missing body: %q", got)
	}
}

func TestFormatRAGSourceCitation(t *testing.T) {
	meta := map[string]any{
		"heading_path":   "Глава 1 > Раздел 2",
		"pdf_page_start": 3,
		"pdf_page_end":   5,
	}

	got := formatRAGSourceCitation(meta)
	if !strings.Contains(got, "раздел=«Глава 1 > Раздел 2»") {
		t.Fatalf("missing heading citation: %q", got)
	}

	if !strings.Contains(got, "стр.=3–5") {
		t.Fatalf("missing page citation: %q", got)
	}
}

func TestInstructionSafeBudgetManager_dropsDocumentContextFirst(t *testing.T) {
	c := &ChatUseCase{
		llmContextFallbackTokens: 512,
	}
	systemPolicy := domain.NewMessage(1, "system", domain.MessageRoleSystem)
	history := []*domain.Message{
		domain.NewMessage(1, "history", domain.MessageRoleAssistant),
	}

	userInstruction := domain.NewMessage(1, "latest user instruction", domain.MessageRoleUser)
	blocks := []documentContextBlock{
		{
			Title:      "RAG-контекст: big.txt",
			Body:       strings.Repeat("A", 800),
			SourceType: "rag",
			SourceFile: "big.txt",
		},
	}

	out, metrics := c.applyInstructionSafeBudgetManager(systemPolicy, history, userInstruction, blocks)
	if len(out) != 0 {
		t.Fatalf("expected context to be dropped first, got %d blocks", len(out))
	}

	if metrics.DroppedRunesTotal == 0 {
		t.Fatal("expected dropped runes metrics")
	}

	if metrics.DroppedRunesByFile["big.txt"] == 0 {
		t.Fatalf("expected by-file metric for big.txt, got %#v", metrics.DroppedRunesByFile)
	}

	if metrics.DroppedRunesBySource["rag"] == 0 {
		t.Fatalf("expected by-source metric for rag, got %#v", metrics.DroppedRunesBySource)
	}
}

func TestSelectMultiFileRAGCandidates_prefersHigherScoresWithLimits(t *testing.T) {
	candidates := []multiFileRAGCandidate{
		{
			fileIndex: 0,
			score:     0.95,
			chunk: domain.ScoredDocumentRAGChunk{
				DocumentRAGChunk: domain.DocumentRAGChunk{
					Text: strings.Repeat("A", 40),
				},
				Score: 0.95,
			},
		},
		{
			fileIndex: 0,
			score:     0.90,
			chunk: domain.ScoredDocumentRAGChunk{
				DocumentRAGChunk: domain.DocumentRAGChunk{
					Text: strings.Repeat("B", 40),
				},
				Score: 0.90,
			},
		},
		{
			fileIndex: 0,
			score:     0.10,
			chunk: domain.ScoredDocumentRAGChunk{
				DocumentRAGChunk: domain.DocumentRAGChunk{
					Text: strings.Repeat("C", 40),
				},
				Score: 0.10,
			},
		},
		{
			fileIndex: 1,
			score:     0.80,
			chunk: domain.ScoredDocumentRAGChunk{
				DocumentRAGChunk: domain.DocumentRAGChunk{
					Text: strings.Repeat("D", 40),
				},
				Score: 0.80,
			},
		},
	}

	out := selectMultiFileRAGCandidates(candidates, 140, 120)
	if len(out[0]) == 0 {
		t.Fatal("expected top chunk for file 0")
	}

	if len(out[1]) == 0 {
		t.Fatal("expected selected chunk for file 1")
	}

	if len(out[0]) > multiFileRAGPerFileLimit {
		t.Fatalf("per-file limit exceeded: %d", len(out[0]))
	}

	if out[0][0].Score < out[1][0].Score {
		t.Fatalf("expected higher score to stay prioritized, file0=%v file1=%v", out[0][0].Score, out[1][0].Score)
	}
}

func TestInstructionSafeBudgetManager_keepsStrictFormatInstructionWithLongDocument(t *testing.T) {
	c := &ChatUseCase{
		llmContextFallbackTokens: 512,
	}

	systemPolicy := domain.NewMessage(1, "system policy", domain.MessageRoleSystem)
	history := []*domain.Message{
		domain.NewMessage(1, "history", domain.MessageRoleAssistant),
	}

	strictInstruction := `Ответь строго JSON-объектом {"status":"ok","items":[]}.`
	userInstruction := domain.NewMessage(1, strictInstruction, domain.MessageRoleUser)
	blocks := []documentContextBlock{
		{
			Title:      "RAG-контекст: long.txt",
			Body:       strings.Repeat("L", 4000),
			SourceType: "rag",
			SourceFile: "long.txt",
		},
	}

	trimmedBlocks, _ := c.applyInstructionSafeBudgetManager(systemPolicy, history, userInstruction, blocks)
	messages := assemblePromptMessages(1, systemPolicy, history, userInstruction, trimmedBlocks)
	last := messages[len(messages)-1]
	if last.Role != domain.MessageRoleUser || last.Content != strictInstruction {
		t.Fatalf("strict user instruction must stay unchanged, got role=%s content=%q", last.Role, last.Content)
	}
}

func TestSelectMultiFileRAGCandidates_respectsBudgetInConflictingMultiFileCase(t *testing.T) {
	var candidates []multiFileRAGCandidate
	for i := range 6 {
		candidates = append(candidates, multiFileRAGCandidate{
			fileIndex: i % 2,
			score:     1.0 - float64(i)*0.05,
			chunk: domain.ScoredDocumentRAGChunk{
				DocumentRAGChunk: domain.DocumentRAGChunk{
					Text: strings.Repeat(string(rune('a'+i)), 90),
				},
				Score: 1.0 - float64(i)*0.05,
			},
		})
	}

	selected := selectMultiFileRAGCandidates(candidates, 220, 120)
	totalSelected := 0
	for _, rows := range selected {
		totalSelected += len(rows)
	}

	if totalSelected == 0 {
		t.Fatal("expected at least one selected fragment")
	}

	if totalSelected > 2 {
		t.Fatalf("expected budget-constrained selection, got %d fragments", totalSelected)
	}
}
