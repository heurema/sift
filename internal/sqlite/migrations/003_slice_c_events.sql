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
    importance_score REAL NOT NULL,
    market_relevance_score REAL NOT NULL,
    confidence_score REAL NOT NULL,
    source_cluster_size INTEGER NOT NULL,
    event_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS event_articles (
    event_id TEXT NOT NULL,
    article_id TEXT NOT NULL,
    source TEXT NOT NULL,
    url TEXT NOT NULL,
    published_at TEXT NOT NULL,
    editorial_type TEXT NOT NULL,
    PRIMARY KEY (event_id, article_id),
    FOREIGN KEY (event_id) REFERENCES events(event_id),
    FOREIGN KEY (article_id) REFERENCES articles(article_id)
);

CREATE INDEX IF NOT EXISTS idx_events_latest ON events (importance_score DESC, confidence_score DESC, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_published ON events (published_at DESC);
CREATE INDEX IF NOT EXISTS idx_event_articles_event_id ON event_articles (event_id);

ALTER TABLE runs ADD COLUMN events_rebuilt INTEGER NOT NULL DEFAULT 0;
