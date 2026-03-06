# Sift v0 Foundation

Дата: 2026-03-06
Статус: draft

## One-liner

Sift is a human-readable crypto news workspace backed by an agent-native event layer.

## Problem

Crypto news is noisy, repetitive, and structurally weak for automation.

Three common failure modes keep showing up:

1. article feeds overwhelm both humans and agents with duplicates;
2. summaries lose evidence and provenance;
3. systems optimize for either a human dashboard or a machine API, but not both on top of the same truth model.

## Why this project exists

The system should let a person open a clean workspace, filter high-signal crypto events, and hand the same context to an external agent without format loss.

That implies one shared core:

- article ingestion and normalization;
- event clustering;
- entity tagging;
- digest projection;
- rights-aware output policy.

## Product model

### Core objects

1. `Article`
   A normalized source record from RSS, API, or another approved source.

2. `Event`
   A cluster of related articles that describe the same underlying fact pattern.

3. `Entity`
   An asset, protocol, exchange, regulator, person, or company referenced by events.

4. `Digest`
   A rendered slice over events for a scope and time window.

### Key decision

`Event` is the primary unit for retrieval and ranking.

Articles remain important for provenance, but the product should not force humans or agents to reconstruct events from raw article streams.

## Canonical truth

The source of truth is typed JSON.

Why:

- stable IDs;
- explicit timestamps;
- typed filters;
- better delta sync;
- reliable provenance handling;
- cleaner contract for CLI access and any future adapters.

Markdown remains important, but only as a projection layer:

- per-event reading pages;
- human digest pages;
- agent handoff context;
- `llms.txt` discovery.

## Delivery surfaces

### P0

- Human UI
- CLI
- Event JSON records
- Per-event Markdown
- Digest Markdown
- `llms.txt`

### P1

- REST/JSON API
- windowed context pages
- JSON Feed 1.1

### P2

- MCP adapter with narrow retrieval tools

## v0 scope

### Included

- crypto only;
- RSS/API-first ingestion;
- article normalization;
- event clustering;
- entity tagging;
- source provenance;
- rights-aware storage mode;
- human workspace with filters and digest view;
- CLI for local agent access;
- JSON and Markdown output surfaces.

### Excluded

- multi-category expansion;
- trading execution;
- embeddings-first architecture;
- generic web scraping as the main ingestion strategy;
- full-text storage by default;
- MCP in the first shipping slice;
- remote API as the primary access path.

## v0 user experience

### Human operator

- opens a latest crypto feed;
- filters by topic, asset, or event type;
- sees deduplicated event cards instead of raw article flood;
- opens an event page with supporting sources and uncertainty;
- copies a short context block for their own agent.

### Agent client

- runs local CLI commands;
- requests stable `json` output;
- fetches one stable event record;
- reads a compact Markdown projection if needed;
- uses provenance and confidence metadata programmatically.

## Data and policy rules

- Every event must have `event_id`.
- Every event must name at least one supporting article.
- Every article must keep its source and canonical URL.
- Every output must respect the current rights policy.
- Every summary is a derived artifact, not primary evidence.

## Initial source strategy

Start with a narrow registry of high-signal crypto sources that expose official feeds or APIs.

Order of preference:

1. official API
2. official RSS/Atom/JSON feed
3. approved structured source
4. HTML fetch only after rights and terms review

## Roadmap after v0 foundation

1. source registry and licensing matrix
2. collector and normalizer
3. event clusterer
4. human UI shell
5. JSON/Markdown publisher
6. `llms.txt` and context windows
7. remote API
8. MCP adapter

## Success criteria for the first build phase

- the object model is stable enough to implement;
- rights policy is explicit enough to avoid accidental overreach;
- a future agent can discover the repo and its contracts without reading the whole tree;
- human UI and agent delivery are clearly defined as two surfaces over one core;
- the CLI is clear enough to become the first real integration boundary.
