package websearch

import "encoding/json"

type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Source  string `json:"source,omitempty"`
}

func MarshalSearchResults(query string, results []Result) (string, error) {
	type out struct {
		Query   string   `json:"query"`
		Results []Result `json:"results"`
	}

	b, err := json.Marshal(out{Query: query, Results: results})
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func normalizeMaxResults(n int) int {
	if n <= 0 {
		return 20
	}

	if n > 20 {
		return 20
	}

	return n
}
