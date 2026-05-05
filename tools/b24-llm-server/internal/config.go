package internal

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
)

type Config struct {
	Addr       string // адрес b24-llm-server
	RunnerAddr string // адрес gen runner
	Model      string // имя модели для генерации

	AnalyzeCacheDisable bool          // не кэшировать ответы analyze/stream/summarize-batch
	AnalyzeCacheTTL     time.Duration // TTL in-memory кэша при совпадении JSON-тела запроса
	AnalyzeCacheMaxKeys int           // лимит записей; при переполнении после очистки истёкших удаляются произвольные ключи

	MaxAnalyzeBodyBytes        int64 // максимальный размер тела для /analyze и /analyze/stream, 0 - без ограничения размера
	MaxSummarizeBatchBodyBytes int64 // лимит тела для /summarize-batch
	MaxSummarizeBatchItems     int   // максимальное число элементов в items
}

func ServerConfig() Config {
	return Config{
		Addr:       "127.0.0.1:8001",
		RunnerAddr: "0.0.0.0:50052",
		Model:      "Qwen3-8B-Q8_0",

		AnalyzeCacheDisable: false,
		AnalyzeCacheTTL:     2 * time.Minute,
		AnalyzeCacheMaxKeys: 256,

		MaxAnalyzeBodyBytes:        6 << 20,  // 6 MiB
		MaxSummarizeBatchBodyBytes: 20 << 20, // 20 MiB
		MaxSummarizeBatchItems:     32,
	}
}

const (
	maxStopSequenceStrings = 16
	maxStopSequenceRunes   = 128
)

func generationParamsFromWire(g *GenerationWire) *domain.GenerationParams {
	if g == nil {
		return nil
	}

	if g.Temperature == nil && g.MaxTokens == nil {
		return nil
	}

	out := &domain.GenerationParams{}
	if g.Temperature != nil {
		t := *g.Temperature
		out.Temperature = &t
	}

	if g.MaxTokens != nil && *g.MaxTokens > 0 {
		out.MaxTokens = g.MaxTokens
	}

	return out
}

func stopSequencesFromWire(g *GenerationWire) []string {
	if g == nil || len(g.StopSequences) == 0 {
		return nil
	}

	out := make([]string, 0, min(len(g.StopSequences), maxStopSequenceStrings))
	seen := make(map[string]struct{}, maxStopSequenceStrings)
	for _, s := range g.StopSequences {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}

		if utf8.RuneCountInString(s) > maxStopSequenceRunes {
			r := []rune(s)
			s = string(r[:maxStopSequenceRunes])
		}

		if _, ok := seen[s]; ok {
			continue
		}

		seen[s] = struct{}{}
		out = append(out, s)
		if len(out) >= maxStopSequenceStrings {
			break
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}
