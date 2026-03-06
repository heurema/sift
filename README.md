# Sift

> Signal over noise for humans and agents.

Sift is a docs-first project for a crypto news system that serves both people and AI agents.

The core idea is simple:

- humans need a fast, deduplicated workspace with filters and digest views;
- agents need typed event records, provenance, stable IDs, and compact Markdown context;
- the system should treat `JSON` as the canonical truth and `Markdown` as a projection for reading and handoff.
- local agents should be able to work through a simple `CLI` without needing MCP.

## Why this exists

Crypto news is fragmented, repetitive, and hard to consume reliably in automation.

Most existing products optimize for one of two things:

- a human dashboard with weak machine contracts;
- a feed or API with weak event modeling and weak provenance.

Sift aims at the gap between them:

- cluster many articles into one event;
- preserve source provenance and rights metadata;
- expose the same knowledge layer to a human UI and to agent-native surfaces.

## Product thesis

The main object is not an article. It is an event.

The system should model:

1. `Article` as a normalized source record.
2. `Event` as a clustered fact pattern across multiple articles.
3. `Entity` as an asset, protocol, exchange, regulator, person, or company.
4. `Digest` as a rendered slice over events for a scope and time window.

## Current scope

v0 is intentionally narrow:

- category: `crypto` only;
- ingestion: `RSS/API first`;
- storage default: metadata plus generated summaries, not full text by default;
- delivery: human UI, typed JSON records, CLI, Markdown, `llms.txt`;
- remote API and MCP: later, after the base contracts are stable.

## Design rules

- `JSON` is the source of truth.
- `Markdown` is the reading surface.
- no source without provenance;
- no storage mode without rights policy;
- no category expansion before crypto event modeling is stable.

## Lineage

Sift is informed by local work and adjacent patterns:

- `Herald` as a local-first news intelligence workflow;
- `llms.txt` and Markdown-for-agents patterns for agent delivery;
- protocol-first work in nearby repos that prefer machine-readable contracts over prose-only rules.

## Current status

Docs-first project.

The implementation does not exist yet. This repo currently defines:

- what the system is;
- why it should exist;
- what v0 must and must not do;
- which contracts agents and future services will rely on.

## Docs

- [manifesto.md](manifesto.md)
- [AGENTS.md](AGENTS.md)
- [llms.txt](llms.txt)
- [project.manifest.json](project.manifest.json)
- [docs/README.md](docs/README.md)
- [docs/plans/2026-03-06-sift-v0-execution-spec.md](docs/plans/2026-03-06-sift-v0-execution-spec.md)
