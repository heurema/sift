CREATE TABLE IF NOT EXISTS sources (
    source_id TEXT PRIMARY KEY,
    source_name TEXT NOT NULL,
    source_class TEXT NOT NULL,
    access_method TEXT NOT NULL,
    url TEXT NOT NULL,
    source_weight REAL NOT NULL,
    rights_mode TEXT NOT NULL,
    excerpt_allowed INTEGER NOT NULL CHECK (excerpt_allowed IN (0, 1)),
    summary_allowed INTEGER NOT NULL CHECK (summary_allowed IN (0, 1)),
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
    notes TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sources_weight ON sources (source_weight DESC, source_id);
CREATE INDEX IF NOT EXISTS idx_runs_finished_at ON runs (finished_at DESC);
