package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"sift/internal/article"
	"sift/internal/event"
	"sift/internal/source"
)

func TestStoreMigrateAndSourceFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sift.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	registry := source.Registry{
		Version:   1,
		Category:  "crypto",
		UpdatedAt: "2026-03-06T16:45:00Z",
		Sources: []source.Source{
			{
				SourceID:             "coindesk_rss",
				SourceName:           "CoinDesk",
				SourceClass:          "media",
				AccessMethod:         "rss",
				URL:                  "https://example.com/feed",
				SourceWeight:         1.0,
				RightsMode:           "metadata_plus_excerpt",
				ExcerptAllowed:       true,
				SummaryAllowed:       true,
				DefaultEditorialType: "report",
				ReviewedAt:           "2026-03-06",
				Notes:                "test",
			},
		},
	}

	loaded, err := store.UpsertSources(ctx, registry)
	if err != nil {
		t.Fatalf("UpsertSources returned error: %v", err)
	}
	if loaded != 1 {
		t.Fatalf("unexpected loaded count: %d", loaded)
	}

	sources, err := store.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources returned error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("unexpected source count: %d", len(sources))
	}
	if sources[0].SourceID != "coindesk_rss" {
		t.Fatalf("unexpected source id: %s", sources[0].SourceID)
	}

	run := Run{
		ID:               "run_test_001",
		Mode:             "full",
		Status:           "success",
		StartedAt:        time.Now().UTC(),
		FinishedAt:       time.Now().UTC(),
		SourcesTotal:     1,
		SourcesLoaded:    1,
		SourcesDegraded:  0,
		ArticlesFetched:  1,
		ArticlesInserted: 1,
		ArticlesUpdated:  0,
		EventsRebuilt:    0,
		Notes:            "test",
	}
	if err := store.InsertRun(ctx, run); err != nil {
		t.Fatalf("InsertRun returned error: %v", err)
	}

	var runCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM runs`).Scan(&runCount); err != nil {
		t.Fatalf("count runs returned error: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("unexpected run count: %d", runCount)
	}

	degraded, err := store.CountDegradedSources(ctx, 5)
	if err != nil {
		t.Fatalf("CountDegradedSources returned error: %v", err)
	}
	if degraded != 0 {
		t.Fatalf("unexpected degraded count: %d", degraded)
	}

	firstSeenAt := time.Date(2026, 3, 6, 16, 0, 0, 0, time.UTC)
	rec, err := article.BuildRecord(article.Candidate{
		SourceID:      "coindesk_rss",
		SourceURL:     "https://example.com/news/a?utm_source=test",
		Title:         "A",
		PublishedAt:   firstSeenAt,
		EditorialType: "report",
		RightsMode:    "metadata_plus_excerpt",
	}, firstSeenAt)
	if err != nil {
		t.Fatalf("BuildRecord returned error: %v", err)
	}

	inserted, updated, err := store.UpsertArticles(ctx, []article.Record{rec})
	if err != nil {
		t.Fatalf("UpsertArticles returned error: %v", err)
	}
	if inserted != 1 || updated != 0 {
		t.Fatalf("unexpected upsert counters: inserted=%d updated=%d", inserted, updated)
	}

	rec.Title = "A updated"
	inserted, updated, err = store.UpsertArticles(ctx, []article.Record{rec})
	if err != nil {
		t.Fatalf("UpsertArticles update returned error: %v", err)
	}
	if inserted != 0 || updated != 1 {
		t.Fatalf("unexpected upsert update counters: inserted=%d updated=%d", inserted, updated)
	}

	articleInputs, err := store.ListArticlesForClustering(ctx)
	if err != nil {
		t.Fatalf("ListArticlesForClustering returned error: %v", err)
	}
	if len(articleInputs) != 1 {
		t.Fatalf("unexpected article input count: %d", len(articleInputs))
	}

	events, err := event.BuildRecords(articleInputs, time.Now().UTC())
	if err != nil {
		t.Fatalf("BuildRecords returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("unexpected event count: %d", len(events))
	}

	if err := store.ReplaceEvents(ctx, events); err != nil {
		t.Fatalf("ReplaceEvents returned error: %v", err)
	}

	latest, err := store.ListLatestEvents(ctx, 10)
	if err != nil {
		t.Fatalf("ListLatestEvents returned error: %v", err)
	}
	if len(latest) != 1 {
		t.Fatalf("unexpected latest count: %d", len(latest))
	}

	allEvents, err := store.ListEvents(ctx)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(allEvents) != 1 {
		t.Fatalf("unexpected all events count: %d", len(allEvents))
	}

	gotEvent, found, err := store.GetEvent(ctx, latest[0].EventID)
	if err != nil {
		t.Fatalf("GetEvent returned error: %v", err)
	}
	if !found {
		t.Fatal("expected GetEvent to find event")
	}
	if gotEvent.EventID != latest[0].EventID {
		t.Fatalf("unexpected event id from GetEvent: %s", gotEvent.EventID)
	}
}

func TestRestoreFromBackupKeepsStoreUsableOnCopyFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sift.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	restoreErr := store.restoreFromBackup(filepath.Join(t.TempDir(), "missing-backup.db"))
	if restoreErr == nil {
		t.Fatal("expected restoreFromBackup to fail for missing backup file")
	}

	registry := source.Registry{
		Version:   1,
		Category:  "crypto",
		UpdatedAt: "2026-03-08T10:00:00Z",
		Sources: []source.Source{
			{
				SourceID:             "test_source",
				SourceName:           "Test Source",
				SourceClass:          "media",
				AccessMethod:         "rss",
				URL:                  "https://example.com/feed",
				SourceWeight:         0.8,
				RightsMode:           "metadata_plus_excerpt",
				ExcerptAllowed:       true,
				SummaryAllowed:       true,
				DefaultEditorialType: "report",
				ReviewedAt:           "2026-03-08",
				Notes:                "test",
			},
		},
	}

	if _, err := store.UpsertSources(ctx, registry); err != nil {
		t.Fatalf("store should remain usable after restore failure, got error: %v", err)
	}
}

func TestReopenDatabaseKeepsExistingHandleWhenPragmasFail(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sift.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	store.applyPragmasFn = func(context.Context, *sql.DB) error {
		return errors.New("apply pragmas failed")
	}
	if err := store.reopenDatabase(dbPath); err == nil {
		t.Fatal("expected reopenDatabase to fail when pragmas applier returns error")
	}

	registry := source.Registry{
		Version:   1,
		Category:  "crypto",
		UpdatedAt: "2026-03-08T10:00:00Z",
		Sources: []source.Source{
			{
				SourceID:             "test_source_reopen",
				SourceName:           "Test Source Reopen",
				SourceClass:          "media",
				AccessMethod:         "rss",
				URL:                  "https://example.com/reopen",
				SourceWeight:         0.8,
				RightsMode:           "metadata_plus_excerpt",
				ExcerptAllowed:       true,
				SummaryAllowed:       true,
				DefaultEditorialType: "report",
				ReviewedAt:           "2026-03-08",
				Notes:                "test",
			},
		},
	}

	if _, err := store.UpsertSources(ctx, registry); err != nil {
		t.Fatalf("store should keep original writable handle after reopen failure, got error: %v", err)
	}
}
