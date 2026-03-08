package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"time"

	"sift/internal/article"
	digestproj "sift/internal/digest"
	"sift/internal/event"
	"sift/internal/ingest"
	"sift/internal/source"
	"sift/internal/sqlite"
)

const (
	degradedSourceThreshold = 5
	defaultFeedTimeout      = 20 * time.Second
)

type Mode string

const (
	ModeFull        Mode = "full"
	ModeFetchOnly   Mode = "fetch_only"
	ModeClusterOnly Mode = "cluster_only"
)

type FetchFeedItemsFunc func(ctx context.Context, client *http.Client, src source.Source) ([]ingest.FeedItem, error)
type NewRunIDFunc func(now time.Time) (string, error)
type NowFunc func() time.Time

type Options struct {
	Mode            Mode
	RegistryPath    string
	OutputDir       string
	RetentionWindow time.Duration
	FetchFeedItems  FetchFeedItemsFunc
	NewRunID        NewRunIDFunc
	Now             NowFunc
}

type Summary struct {
	RunID            string
	Mode             string
	Status           string
	SourcesTotal     int
	SourcesLoaded    int
	SourcesSucceeded int
	SourcesFailed    int
	SourcesDegraded  int
	ArticlesFetched  int
	ArticlesInserted int
	ArticlesUpdated  int
	ArticlesSkipped  int
	EventsRebuilt    int
	ImplementedScope string
}

type Store interface {
	UpsertSources(ctx context.Context, registry source.Registry) (int, error)
	MarkSourceFailure(ctx context.Context, sourceID string, at time.Time, failure string) error
	MarkSourceSuccess(ctx context.Context, sourceID string, at time.Time) error
	UpsertArticles(ctx context.Context, records []article.Record) (int, int, error)
	ListArticlesForClustering(ctx context.Context) ([]event.ArticleInput, error)
	ReplaceEvents(ctx context.Context, records []event.Record) error
	ListEvents(ctx context.Context) ([]event.Record, error)
	ApplyRetention(ctx context.Context, cutoff time.Time) error
	CountDegradedSources(ctx context.Context, threshold int) (int, error)
	InsertRun(ctx context.Context, run sqlite.Run) error
}

func RunSync(ctx context.Context, store Store, opts Options) (Summary, error) {
	if store == nil {
		return Summary{}, fmt.Errorf("store is required")
	}

	mode := opts.Mode
	if mode == "" {
		mode = ModeFull
	}
	if !isValidMode(mode) {
		return Summary{}, fmt.Errorf("unsupported sync mode %q", mode)
	}
	if opts.RetentionWindow < 0 {
		return Summary{}, fmt.Errorf("retention window must be >= 0")
	}

	if opts.RegistryPath == "" {
		return Summary{}, fmt.Errorf("registry path is required")
	}

	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = "output"
	}

	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	fetchFeedItems := opts.FetchFeedItems
	if fetchFeedItems == nil {
		fetchFeedItems = ingest.FetchFeedItems
	}

	newRunID := opts.NewRunID
	if newRunID == nil {
		newRunID = defaultRunID
	}

	startedAt := now()

	registry, err := source.LoadSeed(opts.RegistryPath)
	if err != nil {
		return Summary{}, err
	}

	loaded, err := store.UpsertSources(ctx, registry)
	if err != nil {
		return Summary{}, err
	}

	articlesFetched := 0
	articlesInserted := 0
	articlesUpdated := 0
	articlesSkipped := 0
	sourcesSucceeded := 0
	sourcesFailed := 0
	eventsRebuilt := 0

	if mode != ModeClusterOnly {
		httpClient := &http.Client{Timeout: defaultFeedTimeout}
		firstSeenAt := now()
		recordsByCanonicalURL := make(map[string]article.Record)

		for _, src := range registry.Sources {
			items, err := fetchFeedItems(ctx, httpClient, src)
			if err != nil {
				sourcesFailed++
				if markErr := store.MarkSourceFailure(ctx, src.SourceID, now(), err.Error()); markErr != nil {
					return Summary{}, markErr
				}
				continue
			}

			sourcesSucceeded++
			if err := store.MarkSourceSuccess(ctx, src.SourceID, now()); err != nil {
				return Summary{}, err
			}

			for _, item := range items {
				articlesFetched++

				rec, err := article.BuildRecord(article.Candidate{
					SourceID:      src.SourceID,
					SourceURL:     item.URL,
					Title:         item.Title,
					PublishedAt:   item.Published,
					EditorialType: src.DefaultEditorialType,
					RightsMode:    src.RightsMode,
				}, firstSeenAt)
				if err != nil {
					articlesSkipped++
					continue
				}

				existing, exists := recordsByCanonicalURL[rec.CanonicalURL]
				if exists {
					recordsByCanonicalURL[rec.CanonicalURL] = selectPreferredRecord(existing, rec)
					articlesSkipped++
					continue
				}
				recordsByCanonicalURL[rec.CanonicalURL] = rec
			}
		}

		records := make([]article.Record, 0, len(recordsByCanonicalURL))
		for _, rec := range recordsByCanonicalURL {
			records = append(records, rec)
		}
		sort.Slice(records, func(i, j int) bool {
			if records[i].CanonicalURL == records[j].CanonicalURL {
				return records[i].ArticleID < records[j].ArticleID
			}
			return records[i].CanonicalURL < records[j].CanonicalURL
		})

		inserted, updated, err := store.UpsertArticles(ctx, records)
		if err != nil {
			return Summary{}, err
		}
		articlesInserted = inserted
		articlesUpdated = updated
	}

	if mode != ModeFetchOnly {
		if opts.RetentionWindow > 0 {
			cutoff := now().Add(-opts.RetentionWindow)
			if err := store.ApplyRetention(ctx, cutoff); err != nil {
				return Summary{}, err
			}
		}

		articleInputs, err := store.ListArticlesForClustering(ctx)
		if err != nil {
			return Summary{}, err
		}

		eventRecords, err := event.BuildRecords(articleInputs, now())
		if err != nil {
			return Summary{}, err
		}

		if err := store.ReplaceEvents(ctx, eventRecords); err != nil {
			return Summary{}, err
		}
		eventsRebuilt = len(eventRecords)

		recordsForDigest, err := store.ListEvents(ctx)
		if err != nil {
			return Summary{}, err
		}

		if err := digestproj.PublishDefault(recordsForDigest, outputDir, now()); err != nil {
			return Summary{}, err
		}
	}

	degraded, err := store.CountDegradedSources(ctx, degradedSourceThreshold)
	if err != nil {
		return Summary{}, err
	}

	runID, err := newRunID(now())
	if err != nil {
		return Summary{}, err
	}

	run := sqlite.Run{
		ID:               runID,
		Mode:             string(mode),
		Status:           "success",
		StartedAt:        startedAt,
		FinishedAt:       now(),
		SourcesTotal:     len(registry.Sources),
		SourcesLoaded:    loaded,
		SourcesDegraded:  degraded,
		ArticlesFetched:  articlesFetched,
		ArticlesInserted: articlesInserted,
		ArticlesUpdated:  articlesUpdated,
		EventsRebuilt:    eventsRebuilt,
		Notes:            scopeForMode(mode),
	}

	if err := store.InsertRun(ctx, run); err != nil {
		return Summary{}, err
	}

	return Summary{
		RunID:            run.ID,
		Mode:             run.Mode,
		Status:           run.Status,
		SourcesTotal:     run.SourcesTotal,
		SourcesLoaded:    run.SourcesLoaded,
		SourcesSucceeded: sourcesSucceeded,
		SourcesFailed:    sourcesFailed,
		SourcesDegraded:  run.SourcesDegraded,
		ArticlesFetched:  run.ArticlesFetched,
		ArticlesInserted: run.ArticlesInserted,
		ArticlesUpdated:  run.ArticlesUpdated,
		ArticlesSkipped:  articlesSkipped,
		EventsRebuilt:    run.EventsRebuilt,
		ImplementedScope: run.Notes,
	}, nil
}

func selectPreferredRecord(existing, candidate article.Record) article.Record {
	if candidate.PublishedAt < existing.PublishedAt {
		return candidate
	}
	if candidate.PublishedAt > existing.PublishedAt {
		return existing
	}

	if candidate.SourceID < existing.SourceID {
		return candidate
	}
	if candidate.SourceID > existing.SourceID {
		return existing
	}

	if candidate.ArticleID < existing.ArticleID {
		return candidate
	}
	return existing
}

func isValidMode(mode Mode) bool {
	switch mode {
	case ModeFull, ModeFetchOnly, ModeClusterOnly:
		return true
	default:
		return false
	}
}

func scopeForMode(mode Mode) string {
	switch mode {
	case ModeFetchOnly:
		return "slice_b_fetch_normalize_persist"
	case ModeClusterOnly:
		return "slice_c_cluster_only"
	default:
		return "slice_c_fetch_and_cluster"
	}
}

func defaultRunID(now time.Time) (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate run id suffix: %w", err)
	}

	return fmt.Sprintf(
		"run_%s_%s",
		now.UTC().Format("20060102T150405Z"),
		hex.EncodeToString(suffix[:]),
	), nil
}
