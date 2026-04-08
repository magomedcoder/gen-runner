package websearch

import (
	"context"
	"strings"
)

type Searcher interface {
	Search(ctx context.Context, query string) (string, error)
}

type Options struct {
	Enabled              bool
	Provider             string
	BraveAPIKey          string
	GoogleAPIKey         string
	GoogleSearchEngineID string
	YandexUser           string
	YandexKey            string
	MaxResults           int
}

func New(o Options) Searcher {
	if !o.Enabled {
		return nil
	}

	prov := strings.ToLower(strings.TrimSpace(o.Provider))
	switch prov {
	case "", "brave":
		return NewBraveClient(o.BraveAPIKey, o.MaxResults)
	case "google":
		return NewGoogleCSEClient(o.GoogleAPIKey, o.GoogleSearchEngineID, o.MaxResults)
	case "yandex":
		return NewYandexXMLClient(o.YandexUser, o.YandexKey, o.MaxResults)
	case "multi":
		return buildMulti(o)
	default:
		return nil
	}
}

func buildMulti(o Options) Searcher {
	max := o.MaxResults
	var parts []namedPart
	if y := NewYandexXMLClient(o.YandexUser, o.YandexKey, max); y != nil {
		parts = append(parts, namedPart{name: "yandex", s: y})
	}

	if g := NewGoogleCSEClient(o.GoogleAPIKey, o.GoogleSearchEngineID, max); g != nil {
		parts = append(parts, namedPart{name: "google", s: g})
	}

	if b := NewBraveClient(o.BraveAPIKey, max); b != nil {
		parts = append(parts, namedPart{name: "brave", s: b})
	}

	if len(parts) == 0 {
		return nil
	}

	return newMultiSearcher(parts, max)
}
