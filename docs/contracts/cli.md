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
