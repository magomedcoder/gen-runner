package websearch

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const yandexXMLDefaultEndpoint = "https://yandex.com/search/xml"

type YandexXMLClient struct {
	user       string
	key        string
	endpoint   string
	httpClient *http.Client
	maxResults int
}

func NewYandexXMLClient(user, key string, maxResults int) *YandexXMLClient {
	u := strings.TrimSpace(user)
	k := strings.TrimSpace(key)
	if u == "" || k == "" {
		return nil
	}

	return &YandexXMLClient{
		user:     u,
		key:      k,
		endpoint: yandexXMLDefaultEndpoint,
		httpClient: &http.Client{
			Timeout: 35 * time.Second,
		},
		maxResults: normalizeMaxResults(maxResults),
	}
}

type yandexXMLRoot struct {
	XMLName  xml.Name          `xml:"yandexsearch"`
	Response yandexXMLResponse `xml:"response"`
}

type yandexXMLResponse struct {
	Results yandexXMLResults `xml:"results"`
}

type yandexXMLResults struct {
	Grouping yandexXMLGrouping `xml:"grouping"`
}

type yandexXMLGrouping struct {
	Group []yandexXMLGroup `xml:"group"`
}

type yandexXMLGroup struct {
	Doc []yandexXMLDoc `xml:"doc"`
}

type yandexXMLDoc struct {
	URL      string `xml:"url"`
	Title    string `xml:"title"`
	Passages struct {
		Passage []string `xml:"passage"`
	} `xml:"passages"`
}

func (d *yandexXMLDoc) snippet() string {
	if len(d.Passages.Passage) == 0 {
		return ""
	}

	return strings.TrimSpace(strings.Join(d.Passages.Passage, " "))
}

func (c *YandexXMLClient) Search(ctx context.Context, query string) (string, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", fmt.Errorf("пустой поисковый запрос")
	}

	ep := c.endpoint
	if ep == "" {
		ep = yandexXMLDefaultEndpoint
	}

	u, err := url.Parse(ep)
	if err != nil {
		return "", err
	}

	uv := u.Query()
	uv.Set("user", c.user)
	uv.Set("key", c.key)
	uv.Set("query", q)
	uv.Set("l10n", "ru")
	uv.Set("sortby", "rlv")
	uv.Set("groupby", fmt.Sprintf("attr=d.mode=deep.groups-on-page=%d", c.maxResults))
	u.RawQuery = uv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/xml, text/xml, */*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<23))
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Yandex XML HTTP %d: %s", resp.StatusCode, truncateForErr(body, 512))
	}

	var root yandexXMLRoot
	if err := xml.Unmarshal(body, &root); err != nil {
		return "", fmt.Errorf("разбор XML Yandex: %w", err)
	}

	var out []Result
outer:
	for _, g := range root.Response.Results.Grouping.Group {
		for _, doc := range g.Doc {
			title := strings.TrimSpace(doc.Title)
			link := strings.TrimSpace(doc.URL)
			snip := doc.snippet()
			if title == "" && link == "" {
				continue
			}

			out = append(out, Result{Title: title, URL: link, Snippet: snip})
			if len(out) >= c.maxResults {
				break outer
			}
		}
	}

	return MarshalSearchResults(q, out)
}
