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
	YandexEnabled        bool
	GoogleEnabled        bool
	BraveEnabled         bool
}

func New(o Options) Searcher {
	if !o.Enabled {
		return nil
	}

	prov := strings.ToLower(strings.TrimSpace(o.Provider))
	switch prov {
	case "", "brave":
		if !o.BraveEnabled {
			return nil
		}
		return NewBraveClient(o.BraveAPIKey, o.MaxResults)
	case "google":
		if !o.GoogleEnabled {
			return nil
		}
		return NewGoogleCSEClient(o.GoogleAPIKey, o.GoogleSearchEngineID, o.MaxResults)
	case "yandex":
		if !o.YandexEnabled {
			return nil
		}
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
	if o.YandexEnabled {
		if y := NewYandexXMLClient(o.YandexUser, o.YandexKey, max); y != nil {
			parts = append(parts, namedPart{name: "yandex", s: y})
		}
	}

	if o.GoogleEnabled {
		if g := NewGoogleCSEClient(o.GoogleAPIKey, o.GoogleSearchEngineID, max); g != nil {
			parts = append(parts, namedPart{name: "google", s: g})
		}
	}

	if o.BraveEnabled {
		if b := NewBraveClient(o.BraveAPIKey, max); b != nil {
			parts = append(parts, namedPart{name: "brave", s: b})
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return newMultiSearcher(parts, max)
}
