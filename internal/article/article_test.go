package article

import (
	"testing"
	"time"
)

func TestCanonicalizeURLRemovesTrackingParams(t *testing.T) {
	t.Parallel()

	in := "HTTPS://Example.com/path/to/article/?utm_source=x&keep=1&fbclid=abc&z=2"
	got, err := CanonicalizeURL(in)
	if err != nil {
		t.Fatalf("CanonicalizeURL returned error: %v", err)
	}

	want := "https://example.com/path/to/article?keep=1&z=2"
	if got != want {
		t.Fatalf("unexpected canonical url: got=%q want=%q", got, want)
	}
}

func TestBuildRecordGeneratesDeterministicIDAndDefaultsPublishedAt(t *testing.T) {
	t.Parallel()

	firstSeen := time.Date(2026, 3, 6, 16, 0, 0, 0, time.UTC)
	record, err := BuildRecord(Candidate{
		SourceID:      "coindesk_rss",
		SourceURL:     "https://www.coindesk.com/test/?utm_medium=foo",
		Title:         "  BTC ETF Flows Rise  ",
		EditorialType: "report",
		RightsMode:    "metadata_plus_excerpt",
	}, firstSeen)
	if err != nil {
		t.Fatalf("BuildRecord returned error: %v", err)
	}

	if record.ArticleID == "" {
		t.Fatal("article id is empty")
	}
	if record.CanonicalURL != "https://www.coindesk.com/test" {
		t.Fatalf("unexpected canonical url: %s", record.CanonicalURL)
	}
	if record.PublishedAt != firstSeen.Format(time.RFC3339) {
		t.Fatalf("unexpected published_at: %s", record.PublishedAt)
	}
	if record.Title != "BTC ETF Flows Rise" {
		t.Fatalf("unexpected title normalization: %q", record.Title)
	}
}
