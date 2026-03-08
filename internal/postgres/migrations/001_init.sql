CREATE TABLE IF NOT EXISTS sources (
    source_id TEXT PRIMARY KEY,
    source_name TEXT NOT NULL,
    source_class TEXT NOT NULL,
    access_method TEXT NOT NULL,
    url TEXT NOT NULL,
    source_weight DOUBLE PRECISION NOT NULL,
    rights_mode TEXT NOT NULL,
    excerpt_allowed BOOLEAN NOT NULL,
    summary_allowed BOOLEAN NOT NULL,
    default_editorial_type TEXT NOT NULL,
    reviewed_at TEXT NOT NULL,
    notes TEXT NOT NULL,
    registry_updated_at TEXT NOT NULL,
    last_success_at TEXT,
    last_failure_at TEXT,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
    run_id TEXT PRIMARY KEY,
    mode TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL,
    sources_total INTEGER NOT NULL,
    sources_loaded INTEGER NOT NULL,
    sources_degraded INTEGER NOT NULL DEFAULT 0,
    articles_fetched INTEGER NOT NULL DEFAULT 0,
    articles_inserted INTEGER NOT NULL DEFAULT 0,
    articles_updated INTEGER NOT NULL DEFAULT 0,
    events_rebuilt INTEGER NOT NULL DEFAULT 0,
    notes TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sources_weight ON sources (source_weight DESC, source_id);
CREATE INDEX IF NOT EXISTS idx_runs_finished_at ON runs (finished_at DESC);

CREATE TABLE IF NOT EXISTS articles (
    article_id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES sources(source_id),
    source_url TEXT NOT NULL,
    canonical_url TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    published_at TEXT NOT NULL,
    first_seen_at TEXT NOT NULL,
    editorial_type TEXT NOT NULL,
    rights_mode TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_articles_source_id ON articles (source_id, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_published_at ON articles (published_at DESC);

CREATE TABLE IF NOT EXISTS events (
    event_id TEXT PRIMARY KEY,
    category TEXT NOT NULL,
    status TEXT NOT NULL,
    event_type TEXT NOT NULL,
    title TEXT NOT NULL,
    published_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    first_seen_at TEXT NOT NULL,
    last_verified_at TEXT NOT NULL,
    importance_score DOUBLE PRECISION NOT NULL,
    market_relevance_score DOUBLE PRECISION NOT NULL,
    confidence_score DOUBLE PRECISION NOT NULL,
    source_cluster_size INTEGER NOT NULL,
    event_json JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS event_articles (
    event_id TEXT NOT NULL REFERENCES events(event_id),
    article_id TEXT NOT NULL REFERENCES articles(article_id),
    source TEXT NOT NULL,
    url TEXT NOT NULL,
    published_at TEXT NOT NULL,
    editorial_type TEXT NOT NULL,
    PRIMARY KEY (event_id, article_id)
);

CREATE INDEX IF NOT EXISTS idx_events_latest ON events (importance_score DESC, confidence_score DESC, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_published ON events (published_at DESC);
CREATE INDEX IF NOT EXISTS idx_event_articles_event_id ON event_articles (event_id);
