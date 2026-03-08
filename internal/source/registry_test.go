package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSeedValid(t *testing.T) {
	t.Parallel()

	path := writeTempRegistry(
		t,
		`{
  "version": 1,
  "category": "crypto",
  "updated_at": "2026-03-06T16:45:00Z",
  "sources": [
    {
      "source_id": "coindesk_rss",
      "source_name": "CoinDesk",
      "source_class": "media",
      "access_method": "rss",
      "url": "https://example.com/feed",
      "source_weight": 1.0,
      "rights_mode": "metadata_plus_excerpt",
      "excerpt_allowed": true,
      "summary_allowed": true,
      "default_editorial_type": "report",
      "reviewed_at": "2026-03-06",
      "notes": "ok"
    }
  ]
}`,
	)

	registry, err := LoadSeed(path)
	if err != nil {
		t.Fatalf("LoadSeed returned error: %v", err)
	}

	if registry.Version != 1 {
		t.Fatalf("unexpected registry version: %d", registry.Version)
	}
	if len(registry.Sources) != 1 {
		t.Fatalf("unexpected source count: %d", len(registry.Sources))
	}
	if registry.Sources[0].SourceID != "coindesk_rss" {
		t.Fatalf("unexpected source id: %s", registry.Sources[0].SourceID)
	}
}

func TestLoadSeedRejectsDuplicateSourceID(t *testing.T) {
	t.Parallel()

	path := writeTempRegistry(
		t,
		`{
  "version": 1,
  "category": "crypto",
  "updated_at": "2026-03-06T16:45:00Z",
  "sources": [
    {
      "source_id": "dup",
      "source_name": "A",
      "source_class": "media",
      "access_method": "rss",
      "url": "https://example.com/a",
      "source_weight": 1.0,
      "rights_mode": "metadata_plus_excerpt",
      "excerpt_allowed": true,
      "summary_allowed": true,
      "default_editorial_type": "report",
      "reviewed_at": "2026-03-06",
      "notes": "a"
    },
    {
      "source_id": "dup",
      "source_name": "B",
      "source_class": "media",
      "access_method": "rss",
      "url": "https://example.com/b",
      "source_weight": 0.9,
      "rights_mode": "metadata_plus_excerpt",
      "excerpt_allowed": true,
      "summary_allowed": true,
      "default_editorial_type": "report",
      "reviewed_at": "2026-03-06",
      "notes": "b"
    }
  ]
}`,
	)

	if _, err := LoadSeed(path); err == nil {
		t.Fatal("expected LoadSeed to fail on duplicate source_id")
	}
}

func writeTempRegistry(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp registry: %v", err)
	}
	return path
}
