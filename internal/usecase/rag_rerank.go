package usecase

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
)

const ragRerankSystem = `Ты ранжируешь текстовые фрагменты по релевантности к вопросу пользователя.

Правила:
- Фрагменты пронумерованы 1..N в сообщении пользователя в заданном порядке.
- Выведи ТОЛЬКО числа от 1 до N через запятую (сначала лучший, в конце худший).
- Каждое число от 1 до N должно встретиться ровно один раз.
- Без пояснений, лишних слов и markdown.`

var rerankDigitSplit = regexp.MustCompile(`[^0-9]+`)

func trimPassageForRerank(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || s == "" {
		return s
	}

	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}

	r := []rune(s)
	return string(r[:maxRunes]) + "…"
}

func parseRerankOrder(reply string, n int) []int {
	if n <= 0 {
		return nil
	}
	reply = strings.TrimSpace(reply)
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)

	var raw []string
	if strings.Contains(reply, ",") {
		for p := range strings.SplitSeq(reply, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}

			raw = append(raw, p)
		}
	} else {
		for _, tok := range rerankDigitSplit.Split(reply, -1) {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}

			raw = append(raw, tok)
		}
	}

	seen := make(map[int]struct{})
	var order []int
	for _, p := range raw {
		v, err := strconv.Atoi(p)
		if err != nil || v < 1 || v > n {
			continue
		}

		if _, ok := seen[v]; ok {
			continue
		}

		seen[v] = struct{}{}
		order = append(order, v-1)
	}

	if len(order) < n {
		for i := range n {
			if _, ok := seen[i+1]; !ok {
				order = append(order, i)
			}
		}
	}

	if len(order) > n {
		order = order[:n]
	}

	return order
}

func (u *DocumentIngestUseCase) rerankSearchHits(
	ctx context.Context,
	sessionID int64,
	userQuery string,
	hits []domain.ScoredDocumentRAGChunk,
	chatModel string,
) ([]domain.ScoredDocumentRAGChunk, int64, error) {
	t0 := time.Now()
	if len(hits) < 2 {
		return hits, 0, nil
	}

	maxC := u.rerankMaxCandidates
	if maxC < 2 {
		maxC = 16
	}

	pool := hits
	var tail []domain.ScoredDocumentRAGChunk
	if len(pool) > maxC {
		pool = hits[:maxC]
		tail = append([]domain.ScoredDocumentRAGChunk(nil), hits[maxC:]...)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Вопрос:\n%s\n\n", strings.TrimSpace(userQuery))
	for i := range pool {
		excerpt := trimPassageForRerank(pool[i].DocumentRAGChunk.Text, u.rerankPassageMaxRunes)
		fmt.Fprintf(&b, "--- Фрагмент %d ---\n%s\n\n", i+1, excerpt)
	}

	msgs := []*domain.Message{
		domain.NewMessage(sessionID, ragRerankSystem, domain.MessageRoleSystem),
		domain.NewMessage(sessionID, b.String(), domain.MessageRoleUser),
	}

	temp := float32(0.05)
	mt := u.rerankMaxTokens
	gp := &domain.GenerationParams{
		Temperature: &temp,
		MaxTokens:   &mt,
	}

	ch, err := u.llmRepo.SendMessage(ctx, sessionID, chatModel, msgs, nil, u.rerankTimeoutSeconds, gp)
	if err != nil {
		return hits, time.Since(t0).Milliseconds(), err
	}

	var out strings.Builder
	for {
		select {
		case <-ctx.Done():
			return hits, time.Since(t0).Milliseconds(), ctx.Err()
		case chunk, ok := <-ch:
			if !ok {
				goto parsed
			}

			out.WriteString(chunk.Content)
		}
	}
parsed:
	order := parseRerankOrder(out.String(), len(pool))
	ms := time.Since(t0).Milliseconds()
	if len(order) != len(pool) {
		logger.W("DocumentIngest: разбор ответа переупорядочивания RAG: ожидали %d рангов, получили %d ответ=%q", len(pool), len(order), truncateStringRunes(strings.TrimSpace(out.String()), 200))
		return hits, ms, nil
	}

	reordered := make([]domain.ScoredDocumentRAGChunk, 0, len(pool))
	for _, idx := range order {
		if idx < 0 || idx >= len(pool) {
			logger.W("DocumentIngest: переупорядочивание RAG: неверный индекс %d", idx)
			return hits, ms, nil
		}

		reordered = append(reordered, pool[idx])
	}

	return append(reordered, tail...), ms, nil
}
