# CLI Contract

## Purpose

For v0, the primary access path for local AI agents is a CLI, not MCP.

Why:

- agents already work well with shell tools;
- CLI is simpler to version, script, and test;
- it avoids protocol overhead before the event model is stable;
- it works well for local-first and single-host workflows.

MCP is a possible later adapter. It should wrap stable core capabilities, not define them.

## Design rule

The CLI should sit on top of the same canonical event layer used by the human UI and any future remote API.

## Command groups

### Ingestion

- `sift sync`
  Fetch configured sources, normalize articles, cluster events, and refresh derived outputs.

### Retrieval

- `sift latest`
  Show the latest high-signal events.

- `sift search`
  Filter events by asset, topic, event type, status, time window, and source.

- `sift event get <event_id>`
  Return a single event.

- `sift digest <scope>`
  Render a digest for a scope and time window.

### Output modes

Every retrieval command should support:

- `--format json`
- `--format md`
- `--format text`

`json` is the default for agent use.

## Time and filter semantics

- `--since` accepts relative windows like `24h`, `72h`, `7d` or an RFC3339 timestamp.
- `--until` accepts RFC3339 timestamps.
- `--limit` defaults to `20` and must cap at `100`.
- `--asset`, `--topic`, `--event-type`, and `--status` are repeatable filters.

## JSON result envelopes

### List commands

Commands like `sift latest` and `sift search` should return:

```json
{
  "items": [],
  "next_cursor": null,
  "generated_at": "2026-03-06T16:00:00Z"
}
```

### Single event

`sift event get <event_id> --format json` should return one canonical event object that conforms to `event.schema.json`.

### Digest

`sift digest <scope> --format json` should return:

```json
{
  "scope": "crypto",
  "window": "24h",
  "generated_at": "2026-03-06T16:00:00Z",
  "event_ids": [],
  "markdown_path": "output/digests/crypto/24h.md"
}
```

## Exit codes

- `0` success
- `1` operational failure
- `2` invalid arguments
- `3` record not found
- `4` policy block, such as an unapproved source action

## Example commands

```bash
sift sync
sift latest --limit 20 --format json
sift search --asset BTC --topic ETF --since 24h --format json
sift event get evt_2026_03_06_btc_etf_flows_001 --format md
sift digest crypto --window 24h --format md
```

## Stability rules

- Human-readable output may evolve.
- `json` output should be treated as the stable contract.
- When command semantics change, update this file and the event schema together.
