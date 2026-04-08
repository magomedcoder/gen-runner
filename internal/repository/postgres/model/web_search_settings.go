package model

type WebSearchSettings struct {
	ID                   int64  `gorm:"column:id;primaryKey"`
	Enabled              bool   `gorm:"column:enabled"`
	MaxResults           int    `gorm:"column:max_results"`
	BraveAPIKey          string `gorm:"column:brave_api_key"`
	GoogleAPIKey         string `gorm:"column:google_api_key"`
	GoogleSearchEngineID string `gorm:"column:google_search_engine_id"`
	YandexUser           string `gorm:"column:yandex_user"`
	YandexKey            string `gorm:"column:yandex_key"`
}

func (WebSearchSettings) TableName() string {
	return "web_search_settings"
}
