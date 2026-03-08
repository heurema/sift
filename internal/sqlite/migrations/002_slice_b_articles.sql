CREATE TABLE IF NOT EXISTS articles (
    article_id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    source_url TEXT NOT NULL,
    canonical_url TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    published_at TEXT NOT NULL,
    first_seen_at TEXT NOT NULL,
    editorial_type TEXT NOT NULL,
    rights_mode TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (source_id) REFERENCES sources(source_id)
);

CREATE INDEX IF NOT EXISTS idx_articles_source_id ON articles (source_id, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_published_at ON articles (published_at DESC);

ALTER TABLE runs ADD COLUMN articles_fetched INTEGER NOT NULL DEFAULT 0;
ALTER TABLE runs ADD COLUMN articles_inserted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE runs ADD COLUMN articles_updated INTEGER NOT NULL DEFAULT 0;
