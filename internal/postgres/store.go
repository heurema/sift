package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"sift/internal/article"
	"sift/internal/event"
	"sift/internal/source"
	"sift/internal/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(dsn string) (*Store, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("postgres dsn is required")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres database: %w", err)
	}
	return nil
}

func (s *Store) UpsertSources(ctx context.Context, registry source.Registry) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin source upsert transaction: %w", err)
	}
	defer tx.Rollback()

	const query = `
INSERT INTO sources (
	source_id,
	source_name,
	source_class,
	access_method,
	url,
	source_weight,
	rights_mode,
	excerpt_allowed,
	summary_allowed,
	default_editorial_type,
	reviewed_at,
	notes,
	registry_updated_at,
	updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT(source_id) DO UPDATE SET
	source_name = excluded.source_name,
	source_class = excluded.source_class,
	access_method = excluded.access_method,
	url = excluded.url,
	source_weight = excluded.source_weight,
	rights_mode = excluded.rights_mode,
	excerpt_allowed = excluded.excerpt_allowed,
	summary_allowed = excluded.summary_allowed,
	default_editorial_type = excluded.default_editorial_type,
	reviewed_at = excluded.reviewed_at,
	notes = excluded.notes,
	registry_updated_at = excluded.registry_updated_at,
	updated_at = excluded.updated_at
`

	now := time.Now().UTC().Format(time.RFC3339)
	for _, src := range registry.Sources {
		if _, err := tx.ExecContext(
			ctx,
			query,
			src.SourceID,
			src.SourceName,
			src.SourceClass,
			src.AccessMethod,
			src.URL,
			src.SourceWeight,
			src.RightsMode,
			src.ExcerptAllowed,
			src.SummaryAllowed,
			src.DefaultEditorialType,
			src.ReviewedAt,
			src.Notes,
			registry.UpdatedAt,
			now,
		); err != nil {
			return 0, fmt.Errorf("upsert source %s: %w", src.SourceID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit source upsert transaction: %w", err)
	}

	return len(registry.Sources), nil
}

func (s *Store) CountDegradedSources(ctx context.Context, threshold int) (int, error) {
	var count int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(1) FROM sources WHERE consecutive_failures >= $1`,
		threshold,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count degraded sources: %w", err)
	}
	return count, nil
}

func (s *Store) InsertRun(ctx context.Context, run sqlite.Run) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO runs (
			run_id,
			mode,
			status,
			started_at,
			finished_at,
			sources_total,
			sources_loaded,
			sources_degraded,
			articles_fetched,
			articles_inserted,
			articles_updated,
			events_rebuilt,
			notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		run.ID,
		run.Mode,
		run.Status,
		run.StartedAt.UTC().Format(time.RFC3339),
		run.FinishedAt.UTC().Format(time.RFC3339),
		run.SourcesTotal,
		run.SourcesLoaded,
		run.SourcesDegraded,
		run.ArticlesFetched,
		run.ArticlesInserted,
		run.ArticlesUpdated,
		run.EventsRebuilt,
		run.Notes,
	)
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	return nil
}

func (s *Store) MarkSourceSuccess(ctx context.Context, sourceID string, at time.Time) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE sources
		SET
			last_success_at = $1,
			consecutive_failures = 0,
			last_error = NULL,
			updated_at = $2
		WHERE source_id = $3`,
		at.UTC().Format(time.RFC3339),
		at.UTC().Format(time.RFC3339),
		sourceID,
	)
	if err != nil {
		return fmt.Errorf("mark source success %s: %w", sourceID, err)
	}
	return nil
}

func (s *Store) MarkSourceFailure(ctx context.Context, sourceID string, at time.Time, failure string) error {
	if len(failure) > 512 {
		failure = failure[:512]
	}

	_, err := s.db.ExecContext(
		ctx,
		`UPDATE sources
		SET
			last_failure_at = $1,
			consecutive_failures = consecutive_failures + 1,
			last_error = $2,
			updated_at = $3
		WHERE source_id = $4`,
		at.UTC().Format(time.RFC3339),
		failure,
		at.UTC().Format(time.RFC3339),
		sourceID,
	)
	if err != nil {
		return fmt.Errorf("mark source failure %s: %w", sourceID, err)
	}
	return nil
}

func (s *Store) UpsertArticles(ctx context.Context, records []article.Record) (int, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("begin article upsert transaction: %w", err)
	}
	defer tx.Rollback()

	insertStmt, err := tx.PrepareContext(
		ctx,
		`INSERT INTO articles (
			article_id,
			source_id,
			source_url,
			canonical_url,
			title,
			published_at,
			first_seen_at,
			editorial_type,
			rights_mode,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT(canonical_url) DO NOTHING`,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("prepare article insert statement: %w", err)
	}
	defer insertStmt.Close()

	updateStmt, err := tx.PrepareContext(
		ctx,
		`UPDATE articles
		SET
			source_url = $1,
			title = $2,
			published_at = $3,
			editorial_type = $4,
			rights_mode = $5,
			updated_at = $6
		WHERE canonical_url = $7`,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("prepare article update statement: %w", err)
	}
	defer updateStmt.Close()

	inserted := 0
	updated := 0
	now := time.Now().UTC().Format(time.RFC3339)

	for _, rec := range records {
		result, err := insertStmt.ExecContext(
			ctx,
			rec.ArticleID,
			rec.SourceID,
			rec.SourceURL,
			rec.CanonicalURL,
			rec.Title,
			rec.PublishedAt,
			rec.FirstSeenAt,
			rec.EditorialType,
			rec.RightsMode,
			now,
		)
		if err != nil {
			return 0, 0, fmt.Errorf("insert article %s: %w", rec.ArticleID, err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return 0, 0, fmt.Errorf("article insert rows affected: %w", err)
		}
		if rowsAffected > 0 {
			inserted++
			continue
		}

		if _, err := updateStmt.ExecContext(
			ctx,
			rec.SourceURL,
			rec.Title,
			rec.PublishedAt,
			rec.EditorialType,
			rec.RightsMode,
			now,
			rec.CanonicalURL,
		); err != nil {
			return 0, 0, fmt.Errorf("update article %s: %w", rec.ArticleID, err)
		}
		updated++
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit article upsert transaction: %w", err)
	}

	return inserted, updated, nil
}

func (s *Store) ListArticlesForClustering(ctx context.Context) ([]event.ArticleInput, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			a.article_id,
			a.source_id,
			s.source_name,
			s.source_class,
			s.source_weight,
			a.source_url,
			a.canonical_url,
			a.title,
			a.published_at,
			a.first_seen_at,
			a.editorial_type,
			a.rights_mode,
			s.excerpt_allowed
		FROM articles a
		INNER JOIN sources s ON s.source_id = a.source_id
		ORDER BY a.published_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query articles for clustering: %w", err)
	}
	defer rows.Close()

	articles := make([]event.ArticleInput, 0)
	for rows.Next() {
		var rec event.ArticleInput
		var publishedAt string
		var firstSeenAt string

		if err := rows.Scan(
			&rec.ArticleID,
			&rec.SourceID,
			&rec.SourceName,
			&rec.SourceClass,
			&rec.SourceWeight,
			&rec.SourceURL,
			&rec.CanonicalURL,
			&rec.Title,
			&publishedAt,
			&firstSeenAt,
			&rec.EditorialType,
			&rec.RightsMode,
			&rec.SourceExcerptOK,
		); err != nil {
			return nil, fmt.Errorf("scan article row for clustering: %w", err)
		}

		rec.PublishedAt, err = time.Parse(time.RFC3339, publishedAt)
		if err != nil {
			return nil, fmt.Errorf("parse article published_at %s: %w", publishedAt, err)
		}

		rec.FirstSeenAt, err = time.Parse(time.RFC3339, firstSeenAt)
		if err != nil {
			return nil, fmt.Errorf("parse article first_seen_at %s: %w", firstSeenAt, err)
		}

		articles = append(articles, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article rows for clustering: %w", err)
	}

	return articles, nil
}

func (s *Store) ReplaceEvents(ctx context.Context, records []event.Record) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin event replace transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM event_articles`); err != nil {
		return fmt.Errorf("delete existing event_articles: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM events`); err != nil {
		return fmt.Errorf("delete existing events: %w", err)
	}

	insertEventStmt, err := tx.PrepareContext(
		ctx,
		`INSERT INTO events (
			event_id,
			category,
			status,
			event_type,
			title,
			published_at,
			updated_at,
			first_seen_at,
			last_verified_at,
			importance_score,
			market_relevance_score,
			confidence_score,
			source_cluster_size,
			event_json
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
	)
	if err != nil {
		return fmt.Errorf("prepare event insert statement: %w", err)
	}
	defer insertEventStmt.Close()

	insertEventArticleStmt, err := tx.PrepareContext(
		ctx,
		`INSERT INTO event_articles (
			event_id,
			article_id,
			source,
			url,
			published_at,
			editorial_type
		) VALUES ($1, $2, $3, $4, $5, $6)`,
	)
	if err != nil {
		return fmt.Errorf("prepare event_articles insert statement: %w", err)
	}
	defer insertEventArticleStmt.Close()

	for _, rec := range records {
		payload, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("marshal event %s json payload: %w", rec.EventID, err)
		}

		if _, err := insertEventStmt.ExecContext(
			ctx,
			rec.EventID,
			rec.Category,
			rec.Status,
			rec.EventType,
			rec.Title,
			rec.PublishedAt,
			rec.UpdatedAt,
			rec.FirstSeenAt,
			rec.LastVerifiedAt,
			rec.ImportanceScore,
			rec.MarketRelevanceScore,
			rec.ConfidenceScore,
			rec.SourceClusterSize,
			payload,
		); err != nil {
			return fmt.Errorf("insert event %s: %w", rec.EventID, err)
		}

		for _, supporting := range rec.SupportingArticles {
			if _, err := insertEventArticleStmt.ExecContext(
				ctx,
				rec.EventID,
				supporting.ArticleID,
				supporting.Source,
				supporting.URL,
				supporting.PublishedAt,
				supporting.EditorialType,
			); err != nil {
				return fmt.Errorf("insert event article %s/%s: %w", rec.EventID, supporting.ArticleID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit event replace transaction: %w", err)
	}

	return nil
}

func (s *Store) ListLatestEvents(ctx context.Context, limit int) ([]event.Record, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT event_json
		FROM events
		ORDER BY importance_score DESC, confidence_score DESC, published_at DESC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query latest events: %w", err)
	}
	defer rows.Close()

	return scanEventPayloadRows(rows)
}

func (s *Store) ListEvents(ctx context.Context) ([]event.Record, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT event_json
		FROM events
		ORDER BY importance_score DESC, confidence_score DESC, published_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	return scanEventPayloadRows(rows)
}

func (s *Store) GetEvent(ctx context.Context, eventID string) (event.Record, bool, error) {
	var payload []byte
	err := s.db.QueryRowContext(
		ctx,
		`SELECT event_json FROM events WHERE event_id = $1`,
		eventID,
	).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return event.Record{}, false, nil
	}
	if err != nil {
		return event.Record{}, false, fmt.Errorf("query event %s: %w", eventID, err)
	}

	rec, err := eventRecordFromJSON(payload)
	if err != nil {
		return event.Record{}, false, err
	}

	return rec, true, nil
}

func scanEventPayloadRows(rows *sql.Rows) ([]event.Record, error) {
	records := make([]event.Record, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan event payload: %w", err)
		}

		rec, err := eventRecordFromJSON(payload)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event payload rows: %w", err)
	}

	return records, nil
}

func eventRecordFromJSON(payload []byte) (event.Record, error) {
	var rec event.Record
	if err := json.Unmarshal(payload, &rec); err != nil {
		return event.Record{}, fmt.Errorf("decode event payload: %w", err)
	}
	return rec, nil
}
