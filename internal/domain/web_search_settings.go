package domain

type WebSearchSettings struct {
	Enabled              bool
	MaxResults           int
	BraveAPIKey          string
	GoogleAPIKey         string
	GoogleSearchEngineID string
	YandexUser           string
	YandexKey            string
	YandexEnabled        bool
	GoogleEnabled        bool
	BraveEnabled         bool
}
