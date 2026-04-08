package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type namedPart struct {
	name string
	s    Searcher
}

type multiSearcher struct {
	parts      []namedPart
	maxResults int
}

func newMultiSearcher(parts []namedPart, maxResults int) *multiSearcher {
	mr := normalizeMaxResults(maxResults)
	return &multiSearcher{parts: parts, maxResults: mr}
}

func (m *multiSearcher) Search(ctx context.Context, query string) (string, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", fmt.Errorf("пустой поисковый запрос")
	}
	if len(m.parts) == 0 {
		return "", fmt.Errorf("multi: нет настроенных провайдеров")
	}

	type payload struct {
		Results []Result `json:"results"`
	}

	var merged []Result
	for _, p := range m.parts {
		raw, err := p.s.Search(ctx, q)
		if err != nil {
			continue
		}

		var pl payload
		if err := json.Unmarshal([]byte(raw), &pl); err != nil {
			continue
		}

		for _, r := range pl.Results {
			r2 := r
			if r2.Source == "" {
				r2.Source = p.name
			}
			merged = append(merged, r2)
		}
	}

	if len(merged) == 0 {
		return "", fmt.Errorf("ни один из провайдеров не вернул результатов")
	}

	merged = dedupeResultsByURL(merged, m.maxResults)
	return MarshalSearchResults(q, merged)
}

func dedupeResultsByURL(in []Result, limit int) []Result {
	seen := make(map[string]struct{})
	out := make([]Result, 0, len(in))
	for _, r := range in {
		key := normalizeURLKey(r.URL)
		if key == "" {
			continue
		}

		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeURLKey(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	u, err := url.Parse(s)
	if err != nil {
		return strings.ToLower(s)
	}

	u.Fragment = ""
	u.RawQuery = ""
	host := strings.ToLower(u.Hostname())
	path := strings.TrimSuffix(u.EscapedPath(), "/")

	return host + path
}
