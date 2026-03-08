package pipeline

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sift/internal/ingest"
	"sift/internal/source"
	"sift/internal/sqlite"
)

func TestRunSyncFullMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)

	registryPath := filepath.Join(t.TempDir(), "registry.json")
	writeRegistryFile(t, registryPath)

	stateDir := filepath.Join(t.TempDir(), "state")
	store, err := sqlite.OpenStateStore(ctx, stateDir)
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer store.Close()

	outputDir := filepath.Join(t.TempDir(), "output")
	summary, err := RunSync(ctx, store, Options{
		Mode:         ModeFull,
		RegistryPath: registryPath,
		OutputDir:    outputDir,
		Now:          func() time.Time { return now },
		NewRunID: func(_ time.Time) (string, error) {
			return "run_test_sync", nil
		},
		FetchFeedItems: func(_ context.Context, _ *http.Client, _ source.Source) ([]ingest.FeedItem, error) {
			return []ingest.FeedItem{
				{
					URL:       "https://example.com/news/one",
					Title:     "SEC approves Bitcoin ETF filing",
					Published: now.Add(-1 * time.Hour),
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunSync returned error: %v", err)
	}

	if summary.RunID != "run_test_sync" {
		t.Fatalf("unexpected run id: %s", summary.RunID)
	}
	if summary.Mode != string(ModeFull) {
		t.Fatalf("unexpected mode: %s", summary.Mode)
	}
	if summary.SourcesSucceeded != 1 || summary.SourcesFailed != 0 {
		t.Fatalf("unexpected source counters: succeeded=%d failed=%d", summary.SourcesSucceeded, summary.SourcesFailed)
	}
	if summary.EventsRebuilt != 1 {
		t.Fatalf("unexpected events rebuilt: %d", summary.EventsRebuilt)
	}

	events, err := store.ListEvents(ctx)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("unexpected event count in store: %d", len(events))
	}

	if _, err := os.Stat(filepath.Join(outputDir, "digests", "crypto", "24h.json")); err != nil {
		t.Fatalf("missing 24h digest projection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "digests", "crypto", "7d.json")); err != nil {
		t.Fatalf("missing 7d digest projection: %v", err)
	}
}

func TestRunSyncFetchOnlyMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)

	registryPath := filepath.Join(t.TempDir(), "registry.json")
	writeRegistryFile(t, registryPath)

	stateDir := filepath.Join(t.TempDir(), "state")
	store, err := sqlite.OpenStateStore(ctx, stateDir)
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer store.Close()

	outputDir := filepath.Join(t.TempDir(), "output")
	summary, err := RunSync(ctx, store, Options{
		Mode:         ModeFetchOnly,
		RegistryPath: registryPath,
		OutputDir:    outputDir,
		Now:          func() time.Time { return now },
		NewRunID: func(_ time.Time) (string, error) {
			return "run_test_fetch_only", nil
		},
		FetchFeedItems: func(_ context.Context, _ *http.Client, _ source.Source) ([]ingest.FeedItem, error) {
			return []ingest.FeedItem{
				{
					URL:       "https://example.com/news/fetch-only",
					Title:     "BTC inflows rise",
					Published: now.Add(-2 * time.Hour),
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunSync returned error: %v", err)
	}

	if summary.Mode != string(ModeFetchOnly) {
		t.Fatalf("unexpected mode: %s", summary.Mode)
	}
	if summary.EventsRebuilt != 0 {
		t.Fatalf("expected no events rebuilt in fetch-only mode, got %d", summary.EventsRebuilt)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "digests", "crypto", "24h.json")); !os.IsNotExist(err) {
		t.Fatalf("digest should not be created in fetch-only mode, stat err=%v", err)
	}
}

func TestRunSyncFetchOnlyPrefersDeterministicDuplicateWinner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)

	registryPath := filepath.Join(t.TempDir(), "registry.json")
	writeRegistryFileWithSources(t, registryPath, []source.Source{
		{
			SourceID:             "source_a",
			SourceName:           "Source A",
			SourceClass:          "media",
			AccessMethod:         "rss",
			URL:                  "https://example.com/a.xml",
			SourceWeight:         0.5,
			RightsMode:           "metadata_plus_excerpt",
			ExcerptAllowed:       true,
			SummaryAllowed:       true,
			DefaultEditorialType: "report",
			ReviewedAt:           "2026-03-08T08:00:00Z",
			Notes:                "A",
		},
		{
			SourceID:             "source_b",
			SourceName:           "Source B",
			SourceClass:          "media",
			AccessMethod:         "rss",
			URL:                  "https://example.com/b.xml",
			SourceWeight:         0.5,
			RightsMode:           "metadata_plus_excerpt",
			ExcerptAllowed:       true,
			SummaryAllowed:       true,
			DefaultEditorialType: "report",
			ReviewedAt:           "2026-03-08T08:00:00Z",
			Notes:                "B",
		},
	})

	stateDir := filepath.Join(t.TempDir(), "state")
	store, err := sqlite.OpenStateStore(ctx, stateDir)
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer store.Close()

	_, err = RunSync(ctx, store, Options{
		Mode:         ModeFetchOnly,
		RegistryPath: registryPath,
		OutputDir:    filepath.Join(t.TempDir(), "output"),
		Now:          func() time.Time { return now },
		NewRunID: func(_ time.Time) (string, error) {
			return "run_test_fetch_dup", nil
		},
		FetchFeedItems: func(_ context.Context, _ *http.Client, src source.Source) ([]ingest.FeedItem, error) {
			switch src.SourceID {
			case "source_a":
				return []ingest.FeedItem{
					{
						URL:       "https://example.com/news/same-url",
						Title:     "Duplicate event from A",
						Published: now.Add(-1 * time.Hour),
					},
				}, nil
			case "source_b":
				return []ingest.FeedItem{
					{
						URL:       "https://example.com/news/same-url",
						Title:     "Duplicate event from B",
						Published: now.Add(-2 * time.Hour),
					},
				}, nil
			default:
				return nil, nil
			}
		},
	})
	if err != nil {
		t.Fatalf("RunSync returned error: %v", err)
	}

	articleInputs, err := store.ListArticlesForClustering(ctx)
	if err != nil {
		t.Fatalf("ListArticlesForClustering returned error: %v", err)
	}
	if len(articleInputs) != 1 {
		t.Fatalf("unexpected article count: %d", len(articleInputs))
	}
	if articleInputs[0].SourceID != "source_b" {
		t.Fatalf("unexpected winning source for duplicate canonical url: %s", articleInputs[0].SourceID)
	}
}

func TestRunSyncClusterOnlyAppliesRetentionBeforeRebuild(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)
	oldNow := now.Add(-45 * 24 * time.Hour)

	registryPath := filepath.Join(t.TempDir(), "registry.json")
	writeRegistryFile(t, registryPath)

	stateDir := filepath.Join(t.TempDir(), "state")
	store, err := sqlite.OpenStateStore(ctx, stateDir)
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer store.Close()

	_, err = RunSync(ctx, store, Options{
		Mode:         ModeFull,
		RegistryPath: registryPath,
		OutputDir:    filepath.Join(t.TempDir(), "output-initial"),
		Now:          func() time.Time { return oldNow },
		NewRunID: func(_ time.Time) (string, error) {
			return "run_old_full", nil
		},
		FetchFeedItems: func(_ context.Context, _ *http.Client, _ source.Source) ([]ingest.FeedItem, error) {
			return []ingest.FeedItem{
				{
					URL:       "https://example.com/news/old",
					Title:     "Old event that should expire",
					Published: oldNow,
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("initial RunSync returned error: %v", err)
	}

	initialEvents, err := store.ListEvents(ctx)
	if err != nil {
		t.Fatalf("ListEvents after initial run returned error: %v", err)
	}
	if len(initialEvents) != 1 {
		t.Fatalf("unexpected initial event count: %d", len(initialEvents))
	}

	summary, err := RunSync(ctx, store, Options{
		Mode:            ModeClusterOnly,
		RegistryPath:    registryPath,
		OutputDir:       filepath.Join(t.TempDir(), "output-retention"),
		RetentionWindow: 30 * 24 * time.Hour,
		Now:             func() time.Time { return now },
		NewRunID: func(_ time.Time) (string, error) {
			return "run_retention_cluster", nil
		},
	})
	if err != nil {
		t.Fatalf("retention RunSync returned error: %v", err)
	}

	if summary.EventsRebuilt != 0 {
		t.Fatalf("expected 0 events rebuilt after retention, got %d", summary.EventsRebuilt)
	}

	articles, err := store.ListArticlesForClustering(ctx)
	if err != nil {
		t.Fatalf("ListArticlesForClustering returned error: %v", err)
	}
	if len(articles) != 0 {
		t.Fatalf("expected retained article set to be empty, got %d", len(articles))
	}

	events, err := store.ListEvents(ctx)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected retained event set to be empty, got %d", len(events))
	}
}

func writeRegistryFile(t *testing.T, path string) {
	t.Helper()

	registry := source.Registry{
		Version:   1,
		Category:  "crypto",
		UpdatedAt: "2026-03-08T09:00:00Z",
		Sources: []source.Source{
			{
				SourceID:             "test_feed",
				SourceName:           "Test Feed",
				SourceClass:          "media",
				AccessMethod:         "rss",
				URL:                  "https://example.com/rss.xml",
				SourceWeight:         0.7,
				RightsMode:           "metadata_plus_excerpt",
				ExcerptAllowed:       true,
				SummaryAllowed:       true,
				DefaultEditorialType: "report",
				ReviewedAt:           "2026-03-08T08:00:00Z",
				Notes:                "test source",
			},
		},
	}

	writeRegistry(t, path, registry)
}

func writeRegistryFileWithSources(t *testing.T, path string, sources []source.Source) {
	t.Helper()

	registry := source.Registry{
		Version:   1,
		Category:  "crypto",
		UpdatedAt: "2026-03-08T09:00:00Z",
		Sources:   sources,
	}

	writeRegistry(t, path, registry)
}

func writeRegistry(t *testing.T, path string, registry source.Registry) {
	t.Helper()

	raw, err := json.Marshal(registry)
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}

	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write registry file: %v", err)
	}
}
