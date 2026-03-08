package digest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sift/internal/event"
)

func TestParseWindowDuration(t *testing.T) {
	t.Parallel()

	d24h, err := parseWindowDuration("24h")
	if err != nil {
		t.Fatalf("parseWindowDuration(24h) returned error: %v", err)
	}
	if d24h != 24*time.Hour {
		t.Fatalf("unexpected 24h duration: %s", d24h)
	}

	d7d, err := parseWindowDuration("7d")
	if err != nil {
		t.Fatalf("parseWindowDuration(7d) returned error: %v", err)
	}
	if d7d != 7*24*time.Hour {
		t.Fatalf("unexpected 7d duration: %s", d7d)
	}
}

func TestParseWindowDurationErrors(t *testing.T) {
	t.Parallel()

	_, err := parseWindowDuration("")
	if !errors.Is(err, ErrWindowValueRequired) {
		t.Fatalf("expected ErrWindowValueRequired, got: %v", err)
	}

	_, err = parseWindowDuration("bad-window")
	if !errors.Is(err, ErrInvalidWindowValue) {
		t.Fatalf("expected ErrInvalidWindowValue, got: %v", err)
	}
}

func TestSelectEvents(t *testing.T) {
	t.Parallel()

	records := []event.Record{
		{
			EventID:     "evt_crypto_recent",
			Category:    "crypto",
			EventType:   "policy",
			Assets:      []string{"BTC"},
			Topics:      []string{"policy"},
			PublishedAt: "2026-03-06T11:00:00Z",
		},
		{
			EventID:     "evt_crypto_old",
			Category:    "crypto",
			EventType:   "etf",
			Assets:      []string{"BTC"},
			Topics:      []string{"etf"},
			PublishedAt: "2026-03-01T11:00:00Z",
		},
		{
			EventID:     "evt_non_crypto",
			Category:    "macro",
			EventType:   "policy",
			Assets:      []string{"USD"},
			Topics:      []string{"rates"},
			PublishedAt: "2026-03-06T11:00:00Z",
		},
	}

	since := time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)
	filtered, err := selectEvents(records, "crypto", since)
	if err != nil {
		t.Fatalf("selectEvents returned error: %v", err)
	}

	if len(filtered) != 1 {
		t.Fatalf("unexpected filtered count: %d", len(filtered))
	}
	if filtered[0].EventID != "evt_crypto_recent" {
		t.Fatalf("unexpected event id: %s", filtered[0].EventID)
	}
}

func TestEventMatchesScope(t *testing.T) {
	t.Parallel()

	rec := event.Record{
		Category:  "crypto",
		EventType: "policy",
		Assets:    []string{"BTC"},
		Topics:    []string{"etf"},
	}

	if !eventMatchesScope(rec, "crypto") {
		t.Fatal("expected scope crypto to match")
	}
	if !eventMatchesScope(rec, "btc") {
		t.Fatal("expected scope btc to match")
	}
	if !eventMatchesScope(rec, "etf") {
		t.Fatal("expected scope etf to match")
	}
	if !eventMatchesScope(rec, "policy") {
		t.Fatal("expected scope policy to match event type")
	}
	if eventMatchesScope(rec, "defi") {
		t.Fatal("expected scope defi not to match")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "output", "digests", "crypto", "24h.json")

	payload := map[string]any{
		"scope": "crypto",
		"count": 1,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}

	if err := writeFileAtomic(path, raw, 0o644); err != nil {
		t.Fatalf("writeFileAtomic returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file failed: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("unexpected file content: %s", string(got))
	}
}

func TestBuildProjection(t *testing.T) {
	t.Parallel()

	generatedAt := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	records := []event.Record{
		{
			EventID:     "evt_recent",
			Category:    "crypto",
			PublishedAt: "2026-03-06T10:30:00Z",
		},
		{
			EventID:     "evt_old",
			Category:    "crypto",
			PublishedAt: "2026-03-04T10:30:00Z",
		},
	}

	projection, err := BuildProjection(records, "output", "crypto", "24h", generatedAt)
	if err != nil {
		t.Fatalf("BuildProjection returned error: %v", err)
	}

	if projection.Envelope.Scope != "crypto" {
		t.Fatalf("unexpected scope: %s", projection.Envelope.Scope)
	}
	if projection.Envelope.Window != "24h" {
		t.Fatalf("unexpected window: %s", projection.Envelope.Window)
	}
	if len(projection.Envelope.EventIDs) != 1 {
		t.Fatalf("unexpected event id count: %d", len(projection.Envelope.EventIDs))
	}
	if projection.Envelope.EventIDs[0] != "evt_recent" {
		t.Fatalf("unexpected first event id: %s", projection.Envelope.EventIDs[0])
	}
	if projection.JSONPath != filepath.Join("output", "digests", "crypto", "24h.json") {
		t.Fatalf("unexpected json path: %s", projection.JSONPath)
	}
	if projection.MarkdownPath != filepath.Join("output", "digests", "crypto", "24h.md") {
		t.Fatalf("unexpected markdown path: %s", projection.MarkdownPath)
	}
}

func TestPublishDefault(t *testing.T) {
	t.Parallel()

	generatedAt := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	records := []event.Record{
		{
			EventID:     "evt_recent",
			Category:    "crypto",
			PublishedAt: "2026-03-06T10:30:00Z",
		},
		{
			EventID:     "evt_mid",
			Category:    "crypto",
			PublishedAt: "2026-03-03T10:30:00Z",
		},
		{
			EventID:     "evt_old",
			Category:    "crypto",
			PublishedAt: "2026-02-20T10:30:00Z",
		},
	}

	outputDir := filepath.Join(t.TempDir(), "output")
	if err := PublishDefault(records, outputDir, generatedAt); err != nil {
		t.Fatalf("PublishDefault returned error: %v", err)
	}

	json24hPath := filepath.Join(outputDir, "digests", "crypto", "24h.json")
	json7dPath := filepath.Join(outputDir, "digests", "crypto", "7d.json")

	payload24h, err := os.ReadFile(json24hPath)
	if err != nil {
		t.Fatalf("read 24h json failed: %v", err)
	}
	payload7d, err := os.ReadFile(json7dPath)
	if err != nil {
		t.Fatalf("read 7d json failed: %v", err)
	}

	var digest24h Envelope
	if err := json.Unmarshal(payload24h, &digest24h); err != nil {
		t.Fatalf("unmarshal 24h json failed: %v", err)
	}
	if len(digest24h.EventIDs) != 1 {
		t.Fatalf("unexpected 24h event count: %d", len(digest24h.EventIDs))
	}

	var digest7d Envelope
	if err := json.Unmarshal(payload7d, &digest7d); err != nil {
		t.Fatalf("unmarshal 7d json failed: %v", err)
	}
	if len(digest7d.EventIDs) != 2 {
		t.Fatalf("unexpected 7d event count: %d", len(digest7d.EventIDs))
	}
}
