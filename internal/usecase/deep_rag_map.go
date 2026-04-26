package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
)

const deepRAGMapSystem = `Ты сжимаешь выдержки из документа в короткую рабочую заметку для финального шага ответа.

Правила:
- По возможности тот же язык, что и у вопроса пользователя.
- Маркированный список или очень короткие абзацы; без вступлений вроде «Вот».
- Только факты и утверждения, подкреплённые выдержками; при необходимости кратко отметь неуверенность.
- Если выдержки почти не помогают вопросу, ответь одной короткой строкой вроде «Мало релевантного в этом блоке.» (язык подстрой под вопрос).`

const deepRAGMaxExcerptRunesPerChunk = 2800

func truncateStringRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}

	if utf8.RuneCountInString(s) <= max {
		return s
	}

	r := []rune(s)
	return string(r[:max]) + "\n...(обрезано)"
}

func formatChunksForDeepMap(batch []domain.ScoredDocumentRAGChunk) string {
	var b strings.Builder
	for _, sc := range batch {
		body := sc.DocumentRAGChunk.Text
		if utf8.RuneCountInString(body) > deepRAGMaxExcerptRunesPerChunk {
			body = truncateStringRunes(body, deepRAGMaxExcerptRunesPerChunk)
		}

		head := sc.DocumentRAGChunk.ChunkIndex
		score := sc.Score
		if score <= ragNeighborOnlyChunkScore/10 {
			fmt.Fprintf(&b, "--- chunk_index=%d (соседний контекст) ---\n%s\n\n", head, body)
		} else {
			fmt.Fprintf(&b, "--- chunk_index=%d близость=%.4f ---\n%s\n\n", head, score, body)
		}
	}
	return b.String()
}

func (c *ChatUseCase) runDeepRAGMapCall(
	ctx context.Context,
	sessionID int64,
	userQuery string,
	excerptBlock string,
	chatModel string,
) (string, error) {
	u := fmt.Sprintf("Вопрос пользователя:\n%s\n\nФрагменты документа:\n%s", strings.TrimSpace(userQuery), strings.TrimSpace(excerptBlock))
	msgs := []*domain.Message{
		domain.NewMessage(sessionID, deepRAGMapSystem, domain.MessageRoleSystem),
		domain.NewMessage(sessionID, u, domain.MessageRoleUser),
	}

	temp := float32(0.2)
	mt := c.deepRAGMapMaxTokens
	gp := &domain.GenerationParams{
		Temperature: &temp,
		MaxTokens:   &mt,
	}

	ch, err := c.llmRepo.SendMessage(ctx, sessionID, chatModel, msgs, nil, c.deepRAGMapTimeoutSeconds, gp)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case chunk, ok := <-ch:
			if !ok {
				return strings.TrimSpace(out.String()), nil
			}
			out.WriteString(chunk.Content)
		}
	}
}

func (c *ChatUseCase) deepRAGMapSummaries(
	ctx context.Context,
	sessionID int64,
	userQuery string,
	scored []domain.ScoredDocumentRAGChunk,
	chatModel string,
) (summary string, mapCalls int, elapsedMs int64, err error) {
	if !c.deepRAGEnabled || len(scored) == 0 {
		return "", 0, 0, nil
	}

	chunksPer := c.deepRAGChunksPerMap
	maxCalls := c.deepRAGMaxMapCalls
	t0 := time.Now()
	var parts []string

	for i := 0; i < len(scored); i += chunksPer {
		if mapCalls >= maxCalls {
			break
		}

		end := min(i+chunksPer, len(scored))

		batch := scored[i:end]
		block := formatChunksForDeepMap(batch)
		if strings.TrimSpace(block) == "" {
			mapCalls++
			continue
		}

		part, callErr := c.runDeepRAGMapCall(ctx, sessionID, userQuery, block, chatModel)
		mapCalls++
		if callErr != nil {
			if errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded) {
				return "", mapCalls, time.Since(t0).Milliseconds(), callErr
			}

			logger.W("ChatUseCase: deep_rag, шаг map: %v", callErr)
			continue
		}

		if p := strings.TrimSpace(part); p != "" {
			parts = append(parts, p)
		}
	}

	joined := strings.Join(parts, "\n\n---\n\n")
	maxOut := c.deepRAGMaxMapOutputRunes
	joined = truncateStringRunes(joined, maxOut)
	elapsedMs = time.Since(t0).Milliseconds()

	return joined, mapCalls, elapsedMs, nil
}
