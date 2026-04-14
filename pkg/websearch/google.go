package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var googleCSEEndpoint = "https://www.googleapis.com/customsearch/v1"

// https://programmablesearchengine.google.com/

type GoogleCSEClient struct {
	apiKey     string
	searchCX   string
	httpClient *http.Client
	maxResults int
}

func NewGoogleCSEClient(apiKey, searchEngineID string, maxResults int) *GoogleCSEClient {
	key := strings.TrimSpace(apiKey)
	cx := strings.TrimSpace(searchEngineID)
	if key == "" || cx == "" {
		return nil
	}

	mr := min(normalizeMaxResults(maxResults), 10)

	return &GoogleCSEClient{
		apiKey:   key,
		searchCX: cx,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxResults: mr,
	}
}

type googleCSEResponse struct {
	Items []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
	} `json:"items"`
}

func (c *GoogleCSEClient) Search(ctx context.Context, query string) (string, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", fmt.Errorf("пустой поисковый запрос")
	}
	u, err := url.Parse(googleCSEEndpoint)
	if err != nil {
		return "", err
	}
	uv := u.Query()
	uv.Set("key", c.apiKey)
	uv.Set("cx", c.searchCX)
	uv.Set("q", q)
	uv.Set("num", strconv.Itoa(c.maxResults))
	u.RawQuery = uv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Google CSE HTTP %d: %s", resp.StatusCode, truncateForErr(body, 512))
	}

	var parsed googleCSEResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("разбор ответа Google CSE: %w", err)
	}

	var out []Result
	for _, it := range parsed.Items {
		title := strings.TrimSpace(it.Title)
		link := strings.TrimSpace(it.Link)
		snip := strings.TrimSpace(it.Snippet)
		if title == "" && link == "" {
			continue
		}

		out = append(out, Result{Title: title, URL: link, Snippet: snip})
	}

	return MarshalSearchResults(q, out)
}
