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

const braveDefaultEndpoint = "https://api.search.brave.com/res/v1/web/search"

type BraveClient struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
	maxResults int
}

func NewBraveClient(apiKey string, maxResults int) *BraveClient {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil
	}
	maxResults = normalizeMaxResults(maxResults)
	return &BraveClient{
		apiKey:   key,
		endpoint: braveDefaultEndpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxResults: maxResults,
	}
}

type braveWebResults struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func (c *BraveClient) Search(ctx context.Context, query string) (string, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", fmt.Errorf("пустой поисковый запрос")
	}

	ep := c.endpoint
	if ep == "" {
		ep = braveDefaultEndpoint
	}
	u, err := url.Parse(ep)
	if err != nil {
		return "", err
	}
	uv := u.Query()
	uv.Set("q", q)
	uv.Set("count", strconv.Itoa(c.maxResults))
	u.RawQuery = uv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", c.apiKey)

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
		return "", fmt.Errorf("Brave Search HTTP %d: %s", resp.StatusCode, truncateForErr(body, 512))
	}

	var parsed braveWebResults
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("разбор ответа Brave: %w", err)
	}

	var out []Result
	for _, r := range parsed.Web.Results {
		title := strings.TrimSpace(r.Title)
		link := strings.TrimSpace(r.URL)
		desc := strings.TrimSpace(r.Description)
		if title == "" && link == "" {
			continue
		}

		out = append(out, Result{Title: title, URL: link, Snippet: desc})
	}

	return MarshalSearchResults(q, out)
}

func truncateForErr(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}

	return s[:n] + "…"
}
