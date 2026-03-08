package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"sift/internal/article"
	"sift/internal/event"
	"sift/internal/source"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	db             *sql.DB
	dbPath         string
	applyPragmasFn func(context.Context, *sql.DB) error
}

type SourceRecord struct {
	SourceID            string  `json:"source_id"`
	SourceName          string  `json:"source_name"`
	RightsMode          string  `json:"rights_mode"`
	LastSuccessAt       *string `json:"last_success_at"`
	LastFailureAt       *string `json:"last_failure_at"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
	LastError           *string `json:"last_error"`
}

type Run struct {
	ID               string
	Mode             string
	Status           string
	StartedAt        time.Time
	FinishedAt       time.Time
	SourcesTotal     int
	SourcesLoaded    int
	SourcesDegraded  int
	ArticlesFetched  int
	ArticlesInserted int
	ArticlesUpdated  int
	EventsRebuilt    int
	Notes            string
}

type migration struct {
	Version int
	Name    string
	SQL     string
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	store := &Store{
		db:             db,
		dbPath:         dbPath,
		applyPragmasFn: applyPragmasOnDB,
	}

	// SQLite works best with a single writer connection in this local-first setup.
	store.db.SetMaxOpenConns(1)

	if err := store.applyPragmas(context.Background()); err != nil {
		store.db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	if err := s.ensureMigrationsTable(ctx); err != nil {
		return err
	}

	migrations, err := readMigrations()
	if err != nil {
		return err
	}

	applied, err := s.appliedVersions(ctx)
	if err != nil {
		return err
	}

	pending := make([]migration, 0, len(migrations))
	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		pending = append(pending, m)
	}

	backupPath := ""
	if len(pending) > 0 {
		backupPath, err = s.createBackup(ctx)
		if err != nil {
			return err
		}
	}

	for _, m := range pending {
		if err := s.applyMigration(ctx, m); err != nil {
			if backupPath != "" {
				if restoreErr := s.restoreFromBackup(backupPath); restoreErr != nil {
					return fmt.Errorf("apply migration %s: %w; restore failed: %v", m.Name, err, restoreErr)
				}
			}
			return fmt.Errorf("apply migration %s: %w", m.Name, err)
		}
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
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			boolToInt(src.ExcerptAllowed),
			boolToInt(src.SummaryAllowed),
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

func (s *Store) ListSources(ctx context.Context) ([]SourceRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT source_id, source_name, rights_mode, last_success_at, last_failure_at, consecutive_failures, last_error
		FROM sources
		ORDER BY source_weight DESC, source_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("query sources: %w", err)
	}
	defer rows.Close()

	sources := make([]SourceRecord, 0)
	for rows.Next() {
		var rec SourceRecord
		var lastSuccess sql.NullString
		var lastFailure sql.NullString
		var lastError sql.NullString

		if err := rows.Scan(
			&rec.SourceID,
			&rec.SourceName,
			&rec.RightsMode,
			&lastSuccess,
			&lastFailure,
			&rec.ConsecutiveFailures,
			&lastError,
		); err != nil {
			return nil, fmt.Errorf("scan source row: %w", err)
		}

		rec.LastSuccessAt = nullableString(lastSuccess)
		rec.LastFailureAt = nullableString(lastFailure)
		rec.LastError = nullableString(lastError)
		sources = append(sources, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source rows: %w", err)
	}

	return sources, nil
}

func (s *Store) CountDegradedSources(ctx context.Context, threshold int) (int, error) {
	var count int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(1) FROM sources WHERE consecutive_failures >= ?`,
		threshold,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count degraded sources: %w", err)
	}
	return count, nil
}

func (s *Store) InsertRun(ctx context.Context, run Run) error {
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
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
			last_success_at = ?,
			consecutive_failures = 0,
			last_error = NULL,
			updated_at = ?
		WHERE source_id = ?`,
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
			last_failure_at = ?,
			consecutive_failures = consecutive_failures + 1,
			last_error = ?,
			updated_at = ?
		WHERE source_id = ?`,
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

	selectExistingStmt, err := tx.PrepareContext(ctx, `SELECT article_id FROM articles WHERE canonical_url = ?`)
	if err != nil {
		return 0, 0, fmt.Errorf("prepare article select statement: %w", err)
	}
	defer selectExistingStmt.Close()

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
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("prepare article insert statement: %w", err)
	}
	defer insertStmt.Close()

	updateStmt, err := tx.PrepareContext(
		ctx,
		`UPDATE articles
		SET
			source_url = ?,
			title = ?,
			published_at = ?,
			editorial_type = ?,
			rights_mode = ?,
			updated_at = ?
		WHERE canonical_url = ?`,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("prepare article update statement: %w", err)
	}
	defer updateStmt.Close()

	inserted := 0
	updated := 0
	now := time.Now().UTC().Format(time.RFC3339)

	for _, rec := range records {
		var existingID string
		err := selectExistingStmt.QueryRowContext(ctx, rec.CanonicalURL).Scan(&existingID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, 0, fmt.Errorf("query existing article by canonical_url: %w", err)
		}

		if errors.Is(err, sql.ErrNoRows) {
			if _, err := insertStmt.ExecContext(
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
			); err != nil {
				return 0, 0, fmt.Errorf("insert article %s: %w", rec.ArticleID, err)
			}
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
			return 0, 0, fmt.Errorf("update article %s: %w", existingID, err)
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
		var excerptAllowed int

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
			&excerptAllowed,
		); err != nil {
			return nil, fmt.Errorf("scan article row for clustering: %w", err)
		}

		rec.SourceExcerptOK = excerptAllowed == 1

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
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
		) VALUES (?, ?, ?, ?, ?, ?)`,
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
			string(payload),
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
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query latest events: %w", err)
	}
	defer rows.Close()

	events := make([]event.Record, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan event payload: %w", err)
		}

		rec, err := eventRecordFromJSON(payload)
		if err != nil {
			return nil, err
		}
		events = append(events, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest events: %w", err)
	}

	return events, nil
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

	events := make([]event.Record, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan event payload: %w", err)
		}

		rec, err := eventRecordFromJSON(payload)
		if err != nil {
			return nil, err
		}
		events = append(events, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}

	return events, nil
}

func (s *Store) GetEvent(ctx context.Context, eventID string) (event.Record, bool, error) {
	var payload string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT event_json FROM events WHERE event_id = ?`,
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

func eventRecordFromJSON(payload string) (event.Record, error) {
	var rec event.Record
	if err := json.Unmarshal([]byte(payload), &rec); err != nil {
		return event.Record{}, fmt.Errorf("decode event payload: %w", err)
	}
	return rec, nil
}

func (s *Store) applyPragmas(ctx context.Context) error {
	return s.applyPragmasApplier()(ctx, s.db)
}

func applyPragmasOnDB(ctx context.Context, db *sql.DB) error {
	queries := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}

	for _, q := range queries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("apply sqlite pragma %q: %w", q, err)
		}
	}

	return nil
}

func (s *Store) ensureMigrationsTable(ctx context.Context) error {
	const query = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	applied_at TEXT NOT NULL
)`

	if _, err := s.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}
	return nil
}

func (s *Store) appliedVersions(ctx context.Context) (map[int]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]struct{})
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan migration version: %w", err)
		}
		applied[version] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration versions: %w", err)
	}

	return applied, nil
}

func (s *Store) applyMigration(ctx context.Context, m migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("execute migration SQL: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		m.Version,
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("record migration version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration transaction: %w", err)
	}

	return nil
}

func (s *Store) createBackup(ctx context.Context) (string, error) {
	_, err := os.Stat(s.dbPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("stat database for backup: %w", err)
	}

	backupDir := filepath.Join(filepath.Dir(s.dbPath), "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}

	backupPath := filepath.Join(
		backupDir,
		fmt.Sprintf("sift-%s.db", time.Now().UTC().Format("20060102T150405Z")),
	)

	// Force pending WAL pages into the main database before file copy backup.
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return "", fmt.Errorf("checkpoint sqlite wal before backup: %w", err)
	}

	if err := copyFile(s.dbPath, backupPath); err != nil {
		return "", fmt.Errorf("create database backup: %w", err)
	}

	return backupPath, nil
}

func (s *Store) restoreFromBackup(backupPath string) error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close database before restore: %w", err)
	}

	if err := copyFile(backupPath, s.dbPath); err != nil {
		if reopenErr := s.reopenDatabase(s.dbPath); reopenErr != nil {
			return fmt.Errorf("restore database backup: %w; reopen original database failed: %v", err, reopenErr)
		}
		return fmt.Errorf("restore database backup: %w", err)
	}

	return s.reopenDatabase(s.dbPath)
}

func (s *Store) reopenDatabase(dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if err := s.applyPragmasApplier()(context.Background(), db); err != nil {
		_ = db.Close()
		return err
	}

	s.db = db
	return nil
}

func (s *Store) applyPragmasApplier() func(context.Context, *sql.DB) error {
	if s.applyPragmasFn == nil {
		return applyPragmasOnDB
	}
	return s.applyPragmasFn
}

func readMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := parseMigrationVersion(entry.Name())
		if err != nil {
			return nil, err
		}

		content, err := fs.ReadFile(migrationsFS, path.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{
			Version: version,
			Name:    entry.Name(),
			SQL:     string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func parseMigrationVersion(fileName string) (int, error) {
	base := strings.TrimSuffix(fileName, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) == 0 || parts[0] == "" {
		return 0, fmt.Errorf("invalid migration filename: %s", fileName)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid migration version in %s: %w", fileName, err)
	}
	return version, nil
}

func copyFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return dst.Sync()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullableString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	value := v.String
	return &value
}
