package event

import (
	"strings"
	"testing"
	"time"
)

func TestBuildRecordsMergesSimilarAssetEvents(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 6, 16, 0, 0, 0, time.UTC)
	records, err := BuildRecords([]ArticleInput{
		{
			ArticleID:       "a1",
			SourceID:        "coindesk_rss",
			SourceName:      "CoinDesk",
			SourceClass:     "media",
			SourceWeight:    1.0,
			SourceURL:       "https://example.com/a1",
			CanonicalURL:    "https://example.com/a1",
			Title:           "Bitcoin ETF inflows hit $500M",
			PublishedAt:     now.Add(-1 * time.Hour),
			FirstSeenAt:     now.Add(-1 * time.Hour),
			EditorialType:   "report",
			RightsMode:      "metadata_plus_excerpt",
			SourceExcerptOK: true,
		},
		{
			ArticleID:       "a2",
			SourceID:        "cointelegraph_rss",
			SourceName:      "Cointelegraph",
			SourceClass:     "media",
			SourceWeight:    0.9,
			SourceURL:       "https://example.com/a2",
			CanonicalURL:    "https://example.com/a2",
			Title:           "Bitcoin ETF inflows hit $500M",
			PublishedAt:     now.Add(-40 * time.Minute),
			FirstSeenAt:     now.Add(-40 * time.Minute),
			EditorialType:   "report",
			RightsMode:      "metadata_plus_excerpt",
			SourceExcerptOK: true,
		},
	}, now)
	if err != nil {
		t.Fatalf("BuildRecords returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	rec := records[0]
	if rec.SourceClusterSize != 2 {
		t.Fatalf("unexpected source cluster size: %d", rec.SourceClusterSize)
	}
	if rec.Status != "multi_source_verified" {
		t.Fatalf("unexpected status: %s", rec.Status)
	}
	if !strings.HasPrefix(rec.EventID, "evt_") {
		t.Fatalf("unexpected event_id format: %s", rec.EventID)
	}
	if len(rec.SupportingArticles) != 2 {
		t.Fatalf("unexpected supporting article count: %d", len(rec.SupportingArticles))
	}
}

func TestBuildRecordsSplitsConflictingAssetEvents(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 6, 16, 0, 0, 0, time.UTC)
	records, err := BuildRecords([]ArticleInput{
		{
			ArticleID:       "a1",
			SourceID:        "coindesk_rss",
			SourceName:      "CoinDesk",
			SourceClass:     "media",
			SourceWeight:    1.0,
			SourceURL:       "https://example.com/a1",
			CanonicalURL:    "https://example.com/a1",
			Title:           "Bitcoin price rises 5%",
			PublishedAt:     now.Add(-1 * time.Hour),
			FirstSeenAt:     now.Add(-1 * time.Hour),
			EditorialType:   "report",
			RightsMode:      "metadata_plus_excerpt",
			SourceExcerptOK: true,
		},
		{
			ArticleID:       "a2",
			SourceID:        "cointelegraph_rss",
			SourceName:      "Cointelegraph",
			SourceClass:     "media",
			SourceWeight:    0.9,
			SourceURL:       "https://example.com/a2",
			CanonicalURL:    "https://example.com/a2",
			Title:           "Ethereum price rises 5%",
			PublishedAt:     now.Add(-50 * time.Minute),
			FirstSeenAt:     now.Add(-50 * time.Minute),
			EditorialType:   "report",
			RightsMode:      "metadata_plus_excerpt",
			SourceExcerptOK: true,
		},
	}, now)
	if err != nil {
		t.Fatalf("BuildRecords returned error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}
