# Sift v0 Execution Spec

Дата: 2026-03-06
Статус: draft

## Purpose

This document closes the implementation ambiguities left open by the foundation doc.

It defines the concrete v0 choices for:

- source registry;
- clustering;
- ranking and scoring;
- storage layout;
- CLI behavior beyond the high-level contract;
- human UI shell.

Anything not specified here is either intentionally deferred or should not be implemented in v0.

## 1) Source registry

The v0 seed registry is fixed in [docs/contracts/source-registry.seed.json](../contracts/source-registry.seed.json).

### v0 source set

The initial source set is exactly:

1. `coindesk_rss`
2. `cointelegraph_rss`
3. `decrypt_feed`
4. `cryptoslate_feed`
5. `bitcoinmagazine_feed`
6. `sec_press_releases`
7. `kraken_blog`

### Explicit non-source for v0

- `CryptoPanic` is a benchmark and comparison surface, not an ingestion source in v0.
- `The Block` is not in the seed registry until rights review is explicit.
- scrape-only sources are out of scope for the first build.

### Rule for adding a source

A new source is not "to be decided by implementation".

It must:

1. be added to the seed registry;
2. declare `rights_mode`;
3. declare a source weight;
4. declare a reviewed date and notes;
5. pass rights review under [docs/policies/source-rights.md](../policies/source-rights.md).

## 2) Canonical storage layout

v0 is local-first and single-node.

### Filesystem layout

```text
state/
  sift.db
  runs/
    {run_id}.json
output/
  events/
    {event_id}.json
    {event_id}.md
  digests/
    crypto/
      24h.json
      24h.md
      7d.json
      7d.md
  latest/
    latest.json
    latest.md
```

### SQLite tables

The initial SQLite layout is fixed as:

1. `sources`
   Registry snapshot for approved sources.

2. `articles`
   One row per normalized article record after URL canonicalization.

3. `article_entities`
   Join table for extracted entities attached to articles.

4. `events`
   Canonical clustered events with scores and summary fields.

5. `event_articles`
   Join table between events and supporting articles.

6. `digests`
   Materialized digest outputs by scope and window.

7. `runs`
   Pipeline runs with timestamps, source counts, and output counts.

### Storage rule

- SQLite is the authoritative local store.
- `output/` is a derived projection layer.
- No second hidden store should exist in v0.

## 3) Article normalization rules

### Canonical URL

Normalization must:

- lowercase the host;
- remove tracking query params like `utm_*`, `fbclid`, `gclid`;
- preserve path and meaningful query fields;
- strip trailing slashes except for root;
- use the normalized URL as the first dedupe key.

### Required article fields

Every stored article must include:

- `article_id`
- `source_id`
- `canonical_url`
- `title`
- `published_at`
- `first_seen_at`
- `editorial_type`
- `rights_mode`

### Editorial type defaults

If a source-specific classifier does not override the value:

- media sources default to `report`
- official sources default to `official_announcement`

## 4) Clustering rules

v0 clustering is deterministic and intentionally simple.

### Step 1: hard dedupe

Articles with the same canonical URL are the same article.

### Step 2: candidate window

Only compare articles if:

- category matches;
- publication time gap is at most `72h`.

### Step 3: merge signals

Two articles are eligible for the same event if:

1. title similarity is `>= 0.82`, and
2. there is at least one overlapping normalized entity or asset.

Alternative merge path:

1. title similarity is `>= 0.90`, and
2. event type inference matches.

### Step 4: merge guards

Do not merge if any of these are true:

- conflicting primary asset sets;
- conflicting hard numbers in title for obvious amount-based events;
- one title indicates rumor/speculation and the other indicates official confirmation, unless the later item is attached as a new supporting article to the same evolving event by a follow-up update path.

### Step 5: canonical event article

The event's canonical article is chosen by:

1. highest source weight;
2. then earliest official source;
3. then earliest publish time.

## 5) Ranking and scoring

### Importance score

`importance_score` is computed in v0 as:

- `0.35 * source_weight_max`
- `0.20 * source_cluster_score`
- `0.20 * recency_score`
- `0.15 * official_confirmation_score`
- `0.10 * entity_impact_score`

All component scores are normalized to `0..1`.

### Confidence score

`confidence_score` starts at `0.35`.

Then add:

- `+0.20` for a second independent source
- `+0.10` for each additional source up to `+0.25`
- `+0.15` if an official source is in the cluster
- `-0.15` if there is unresolved numeric or factual conflict inside the cluster

Clamp to `0..1`.

### Market relevance score

`market_relevance_score` is separate from `importance_score`.

It is computed from:

- asset prominence;
- topic class;
- source type;
- whether the event is market-moving by rule.

Topic defaults:

- `policy`, `enforcement`, `etf`, `exchange`, `security_incident`, `listing`, `delisting`, `capital_flows` are high relevance
- `opinion`, `career`, `general_brand`, `culture` are low relevance

### Latest feed ordering

Default human and CLI latest views sort by:

1. `importance_score` desc
2. `confidence_score` desc
3. `published_at` desc

## 6) CLI behavior

The high-level contract lives in [docs/contracts/cli.md](../contracts/cli.md).

The concrete v0 command set is fixed as:

- `sift sync`
- `sift latest`
- `sift search`
- `sift event get <event_id>`
- `sift digest <scope>`
- `sift sources`

### `sift sources`

This command exists in v0 and returns the approved registry as either:

- `json`
- `text`

### JSON defaults

For agent-facing commands:

- default format is `json`
- default limit is `20`
- list commands always return an envelope with `items`

### No hidden agent mode

There is no special private mode for AI agents.

The same CLI should serve:

- human terminal use;
- shell scripts;
- local coding agents.

## 7) Human UI shell

The human UI is intentionally thin.

### Page set

v0 page set is fixed as:

- `/` redirect or render latest crypto events
- `/latest`
- `/topics/{topic}`
- `/assets/{asset}`
- `/events/{event_id}`

### No auth in v0

The UI is stateless.

There is:

- no login;
- no saved preferences;
- no personal watchlists;
- no server-side sessions.

### Event card fields

Every event card in the latest view must show:

- title
- `event_type`
- primary assets
- publish time
- source cluster size
- importance score band
- confidence score band
- top supporting sources

### Event detail page

The event detail page must include:

- one-line summary
- five-line summary if available
- supporting article list
- affected entities
- uncertainty section
- copy actions for `event_id`, JSON path, and Markdown path

## 8) Deferred, not undefined

These are intentionally not solved in v0:

- remote API auth
- background scheduling model beyond local sync
- cross-category taxonomy
- embeddings and semantic retrieval
- MCP tool surface

The implementation agent should treat these as deferred, not as missing requirements to fill in ad hoc.

