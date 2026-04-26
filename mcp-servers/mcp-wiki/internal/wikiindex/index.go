package wikiindex

import (
	"maps"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type InputChunk struct {
	FilePath   string
	FileName   string
	ChunkIndex int
	Text       string
	Metadata   map[string]any
}

type ChunkRef struct {
	FilePath   string
	FileName   string
	ChunkIndex int
	Text       string
	Metadata   map[string]any
}

type SearchResult struct {
	Chunk        ChunkRef
	Score        float64
	MatchedTerms []string
}

type Stats struct {
	SourceDir       string
	IndexedAt       time.Time
	Files           int
	Chunks          int
	UniqueTerms     int
	RetrievalMethod string
	TokenLocale     string
	Stemming        string
}

type BuildOptions struct {
	TokenLocale string
}

type chunkVector struct {
	ref      ChunkRef
	termFreq map[string]float64
	norm     float64
}

type Index struct {
	mu          sync.RWMutex
	source      string
	indexed     time.Time
	vectors     []chunkVector
	idf         map[string]float64
	fileSeen    map[string]struct{}
	tokenLocale string
}

func New() *Index {
	return &Index{
		idf:      map[string]float64{},
		fileSeen: map[string]struct{}{},
	}
}

func (i *Index) Build(sourceDir string, chunks []InputChunk, opts ...BuildOptions) Stats {
	i.mu.Lock()
	defer i.mu.Unlock()

	loc := ""
	if len(opts) > 0 {
		loc = normalizeLocale(opts[0].TokenLocale)
	}
	i.tokenLocale = loc

	i.source = sourceDir
	i.indexed = time.Now().UTC()
	i.vectors = nil
	i.idf = map[string]float64{}
	i.fileSeen = map[string]struct{}{}

	if len(chunks) == 0 {
		return i.statsLocked()
	}

	type rawVector struct {
		ref    ChunkRef
		counts map[string]int
		total  int
	}

	raw := make([]rawVector, 0, len(chunks))
	docFreq := map[string]int{}

	for _, ch := range chunks {
		terms := tokenizeFiltered(ch.Text, loc)
		if len(terms) == 0 {
			continue
		}

		i.fileSeen[ch.FilePath] = struct{}{}

		counts := map[string]int{}
		for _, t := range terms {
			counts[t]++
		}

		seenInDoc := map[string]struct{}{}
		for term := range counts {
			if _, ok := seenInDoc[term]; ok {
				continue
			}
			docFreq[term]++
			seenInDoc[term] = struct{}{}
		}

		raw = append(raw, rawVector{
			ref: ChunkRef{
				FilePath:   ch.FilePath,
				FileName:   ch.FileName,
				ChunkIndex: ch.ChunkIndex,
				Text:       ch.Text,
				Metadata:   cloneMeta(ch.Metadata),
			},
			counts: counts,
			total:  len(terms),
		})
	}

	nDocs := float64(len(raw))
	if nDocs == 0 {
		return i.statsLocked()
	}

	for term, df := range docFreq {
		i.idf[term] = math.Log((nDocs+1)/(float64(df)+1)) + 1
	}

	i.vectors = make([]chunkVector, 0, len(raw))
	for _, rv := range raw {
		tf := map[string]float64{}
		var normSq float64
		for term, cnt := range rv.counts {
			weight := (float64(cnt) / float64(rv.total)) * i.idf[term]
			tf[term] = weight
			normSq += weight * weight
		}

		i.vectors = append(i.vectors, chunkVector{
			ref:      rv.ref,
			termFreq: tf,
			norm:     math.Sqrt(normSq),
		})
	}

	return i.statsLocked()
}

func (i *Index) Search(query string, topK int) []SearchResult {
	if topK <= 0 {
		topK = 5
	}

	qTerms := tokenizeFiltered(query, i.tokenLocale)
	if len(qTerms) == 0 {
		return nil
	}

	i.mu.RLock()
	defer i.mu.RUnlock()

	if len(i.vectors) == 0 || len(i.idf) == 0 {
		return nil
	}

	qCounts := map[string]int{}
	for _, t := range qTerms {
		if _, ok := i.idf[t]; !ok {
			continue
		}
		qCounts[t]++
	}
	if len(qCounts) == 0 {
		return nil
	}

	var qNormSq float64
	qWeights := map[string]float64{}
	for term, cnt := range qCounts {
		w := (float64(cnt) / float64(len(qTerms))) * i.idf[term]
		qWeights[term] = w
		qNormSq += w * w
	}
	qNorm := math.Sqrt(qNormSq)
	if qNorm == 0 {
		return nil
	}

	out := make([]SearchResult, 0, topK)
	for _, vec := range i.vectors {
		if vec.norm == 0 {
			continue
		}

		var dot float64
		var matched []string
		for term, qw := range qWeights {
			cw, ok := vec.termFreq[term]
			if !ok {
				continue
			}
			dot += qw * cw
			matched = append(matched, term)
		}
		if dot <= 0 || len(matched) == 0 {
			continue
		}

		sort.Strings(matched)

		score := dot / (qNorm * vec.norm)
		out = append(out, SearchResult{
			Chunk:        vec.ref,
			Score:        score,
			MatchedTerms: matched,
		})
	}

	sort.Slice(out, func(a, b int) bool {
		if out[a].Score == out[b].Score {
			if out[a].Chunk.FilePath == out[b].Chunk.FilePath {
				return out[a].Chunk.ChunkIndex < out[b].Chunk.ChunkIndex
			}
			return out[a].Chunk.FilePath < out[b].Chunk.FilePath
		}

		return out[a].Score > out[b].Score
	})

	if len(out) > topK {
		out = out[:topK]
	}
	return out
}

func (i *Index) Stats() Stats {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.statsLocked()
}

func (i *Index) statsLocked() Stats {
	return Stats{
		SourceDir:       i.source,
		IndexedAt:       i.indexed,
		Files:           len(i.fileSeen),
		Chunks:          len(i.vectors),
		UniqueTerms:     len(i.idf),
		RetrievalMethod: "lexical_tfidf",
		TokenLocale:     i.tokenLocale,
		Stemming:        stemStatsLabel(i.tokenLocale),
	}
}

func (i *Index) TokenLocale() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.tokenLocale
}

func TokenizeForSearch(query string, locale string) []string {
	return tokenizeFiltered(query, normalizeLocale(locale))
}

func tokenizeFiltered(s string, locale string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}

	parts := strings.FieldsFunc(s, func(r rune) bool {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return false
		}
		return true
	})

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len([]rune(p)) < 2 {
			continue
		}

		out = append(out, p)
	}

	out = filterStopwords(locale, out)
	if len(out) == 0 {
		return nil
	}

	return applySnowballStem(locale, out)
}

func cloneMeta(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}

	out := make(map[string]any, len(in))
	maps.Copy(out, in)

	return out
}
