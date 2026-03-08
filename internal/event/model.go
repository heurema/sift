package event

import "time"

type ArticleInput struct {
	ArticleID       string
	SourceID        string
	SourceName      string
	SourceClass     string
	SourceWeight    float64
	SourceURL       string
	CanonicalURL    string
	Title           string
	PublishedAt     time.Time
	FirstSeenAt     time.Time
	EditorialType   string
	RightsMode      string
	SourceExcerptOK bool
}

type Entity struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SupportingArticle struct {
	ArticleID     string `json:"article_id"`
	Source        string `json:"source"`
	URL           string `json:"url"`
	PublishedAt   string `json:"published_at"`
	EditorialType string `json:"editorial_type"`
}

type Rights struct {
	StorageMode    string `json:"storage_mode"`
	FullTextStored bool   `json:"full_text_stored"`
	ExcerptAllowed bool   `json:"excerpt_allowed"`
}

type Record struct {
	EventID              string              `json:"event_id"`
	Category             string              `json:"category"`
	Status               string              `json:"status"`
	EventType            string              `json:"event_type"`
	Title                string              `json:"title"`
	Summary1L            string              `json:"summary_1l"`
	Summary5L            []string            `json:"summary_5l,omitempty"`
	Assets               []string            `json:"assets"`
	Topics               []string            `json:"topics"`
	Entities             []Entity            `json:"entities"`
	PublishedAt          string              `json:"published_at"`
	UpdatedAt            string              `json:"updated_at"`
	FirstSeenAt          string              `json:"first_seen_at"`
	LastVerifiedAt       string              `json:"last_verified_at"`
	ImportanceScore      float64             `json:"importance_score"`
	MarketRelevanceScore float64             `json:"market_relevance_score,omitempty"`
	ConfidenceScore      float64             `json:"confidence_score"`
	SourceClusterSize    int                 `json:"source_cluster_size"`
	SupportingArticles   []SupportingArticle `json:"supporting_articles"`
	Rights               Rights              `json:"rights"`
	MarkdownURL          string              `json:"markdown_url,omitempty"`
}
