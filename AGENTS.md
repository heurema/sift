## Project Notes

- This repo is docs-first and contract-first.
- `JSON` is the canonical source of truth. `Markdown` is a projection for reading and agent handoff.
- The system models `articles`, `events`, `entities`, and `digests`. Do not collapse everything back into a feed-only design.
- Provenance is mandatory. Every event-facing claim must be traceable to one or more supporting source records.
- Rights discipline is mandatory. Do not add ingestion or storage behavior that violates the current source policy.
- v0 scope is narrow: crypto only, RSS/API first, no broad scraping-first expansion.

## Editing Rules

- Keep changes atomic and bounded.
- If you change the event model, update both schema and docs in the same change.
- If you change the CLI surface, update `docs/contracts/cli.md`.
- If you change public API shape, update `docs/contracts/openapi.yaml`.
- If you change agent-facing discovery or key docs, update `llms.txt`.
- Prefer machine-readable artifacts over prose when defining rules, contracts, or policy.

## Truth Boundary

- `docs/contracts/event.schema.json` defines the canonical event shape.
- `docs/contracts/cli.md` defines the primary local access surface for agents.
- `docs/contracts/source-registry.seed.json` defines the approved v0 source set.
- `docs/contracts/openapi.yaml` defines the future remote API boundary.
- `docs/policies/source-rights.md` defines default rights and ingestion behavior.
- `docs/plans/2026-03-06-sift-v0-foundation.md` defines current product scope and v0 boundary.
- `docs/plans/2026-03-06-sift-v0-execution-spec.md` defines the concrete v0 implementation decisions.

## Non-Goals For v0

- general web scraping without source review;
- full-text storage by default;
- multi-category expansion before crypto is stable;
- opaque AI-generated facts without evidence;
- MCP as the primary access path.
