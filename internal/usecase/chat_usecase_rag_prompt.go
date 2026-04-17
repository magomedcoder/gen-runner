package usecase

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
)

func ragFragmentHeadingSuffix(meta map[string]any) string {
	if meta == nil {
		return ""
	}

	hp, _ := meta["heading_path"].(string)
	hp = strings.TrimSpace(hp)
	if hp == "" {
		return ""
	}

	return ", раздел=«" + hp + "»"
}

func intFromMeta(meta map[string]any, key string) (int, bool) {
	if meta == nil {
		return 0, false
	}

	v, ok := meta[key]
	if !ok || v == nil {
		return 0, false
	}

	switch x := v.(type) {
	case int:
		return x, true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	default:
		return 0, false
	}
}

func ragFragmentPDFSuffix(meta map[string]any) string {
	ps, ok1 := intFromMeta(meta, "pdf_page_start")
	pe, ok2 := intFromMeta(meta, "pdf_page_end")
	if !ok1 || !ok2 || ps <= 0 || pe < ps {
		return ""
	}

	if ps == pe {
		return ", стр.=" + strconv.Itoa(ps)
	}

	return ", стр.=" + strconv.Itoa(ps) + "–" + strconv.Itoa(pe)
}

func formatRAGSourceCitation(meta map[string]any) string {
	return ragFragmentHeadingSuffix(meta) + ragFragmentPDFSuffix(meta)
}

func formatRAGFragmentHeader(index int, sc domain.ScoredDocumentRAGChunk) string {
	sfx := formatRAGSourceCitation(sc.Metadata)
	if sc.Score <= ragNeighborOnlyChunkScore/10 {
		return fmt.Sprintf("--- Фрагмент %d (соседний контекст, chunk_index=%d%s) ---\n", index, sc.DocumentRAGChunk.ChunkIndex, sfx)
	}

	return fmt.Sprintf("--- Фрагмент %d (близость=%.3f, chunk_index=%d%s) ---\n", index, sc.Score, sc.DocumentRAGChunk.ChunkIndex, sfx)
}

func buildMessageWithRAG(fileName string, userMessage string, scored []domain.ScoredDocumentRAGChunk, maxContextRunes int, deepMapSummary string) string {
	if maxContextRunes < 200 {
		maxContextRunes = 200
	}

	var b strings.Builder
	intro := "Ниже - наиболее релевантные фрагменты из документа (векторный поиск по запросу). Опирайся на них при ответе; при цитировании указывай номер фрагмента.\n\n"
	b.WriteString(intro)
	total := utf8.RuneCountInString(intro)

	if s := strings.TrimSpace(deepMapSummary); s != "" {
		block := "Промежуточное сжатие фрагментов (map-шаг, дополнительно к полным цитатам ниже):\n\n" + s + "\n\n---\n\n"
		br := utf8.RuneCountInString(block)
		if total+br > maxContextRunes {
			room := maxContextRunes - total - 120
			if room < 80 {
				room = 80
			}

			block = truncateStringRunes(block, room) + "\n\n---\n\n"
			br = utf8.RuneCountInString(block)
		}

		b.WriteString(block)
		total += br
	}

	for i, sc := range scored {
		header := formatRAGFragmentHeader(i+1, sc)
		body := sc.DocumentRAGChunk.Text
		piece := header + body + "\n\n"
		r := utf8.RuneCountInString(piece)
		if total+r > maxContextRunes {
			room := maxContextRunes - total - utf8.RuneCountInString(header)
			if room < 64 {
				break
			}

			br := []rune(body)
			if len(br) > room {
				body = string(br[:room]) + "\n...(обрезано по лимиту контекста)"
			}
			piece = header + body + "\n\n"
		}

		b.WriteString(piece)
		total += utf8.RuneCountInString(piece)
		if total >= maxContextRunes {
			break
		}
	}

	b.WriteString("Файл: «")
	b.WriteString(fileName)
	b.WriteString("»\n\n---\n\n")
	b.WriteString(strings.TrimSpace(userMessage))

	return b.String()
}

func buildRAGContextBlock(fileName string, scored []domain.ScoredDocumentRAGChunk, maxContextRunes int, deepMapSummary string) (documentContextBlock, int) {
	if maxContextRunes < 200 {
		maxContextRunes = 200
	}

	var b strings.Builder
	intro := "Релевантные фрагменты из документа (векторный поиск):\n\n"
	b.WriteString(intro)
	total := utf8.RuneCountInString(intro)
	droppedByBudget := 0

	if s := strings.TrimSpace(deepMapSummary); s != "" {
		block := "Сжатое резюме map-шагов:\n\n" + s + "\n\n---\n\n"
		br := utf8.RuneCountInString(block)
		if total+br > maxContextRunes {
			room := maxContextRunes - total - 120
			if room < 80 {
				room = 80
			}

			block = truncateStringRunes(block, room) + "\n\n---\n\n"
			br = utf8.RuneCountInString(block)
		}

		b.WriteString(block)
		total += br
	}

	for i, sc := range scored {
		header := formatRAGFragmentHeader(i+1, sc)
		body := sc.DocumentRAGChunk.Text
		piece := header + body + "\n\n"
		r := utf8.RuneCountInString(piece)
		if total+r > maxContextRunes {
			room := maxContextRunes - total - utf8.RuneCountInString(header)
			if room < 64 {
				droppedByBudget += len(scored) - i
				break
			}

			br := []rune(body)
			if len(br) > room {
				body = string(br[:room]) + "\n...(обрезано по лимиту контекста)"
				droppedByBudget += len(scored) - i - 1
			}
			piece = header + body + "\n\n"
		}

		b.WriteString(piece)
		total += utf8.RuneCountInString(piece)
		if total >= maxContextRunes {
			droppedByBudget += len(scored) - i - 1
			break
		}
	}

	return documentContextBlock{
		Title:      "RAG-контекст: " + fileName,
		Body:       strings.TrimSpace(b.String()),
		SourceType: "rag",
		SourceFile: fileName,
	}, droppedByBudget
}
