# Docs

## Plans

- [2026-03-06-sift-v0-foundation.md](plans/2026-03-06-sift-v0-foundation.md) — project thesis, v0 boundary, object model, delivery surfaces, and roadmap
- [2026-03-06-sift-v0-execution-spec.md](plans/2026-03-06-sift-v0-execution-spec.md) — concrete v0 decisions for sources, clustering, scoring, storage, CLI, and UI
- [2026-03-07-sift-pro-mvp.md](plans/2026-03-07-sift-pro-mvp.md) — hosted premium slice after v0, now aligned to Postgres and Zitadel
- [2026-03-07-sift-pro-execution-plan.md](plans/2026-03-07-sift-pro-execution-plan.md) — narrow hosted implementation path with Postgres and Zitadel as first-class constraints
- [2026-03-07-sift-pro-slice-1-blueprint.md](plans/2026-03-07-sift-pro-slice-1-blueprint.md) — file-level blueprint for the shared-runtime extraction that prepares for hosted Postgres

## Policies

- [source-rights.md](policies/source-rights.md) — ingestion and output rules for source permissions, storage mode, excerpts, and review

## Contracts

- [event.schema.json](contracts/event.schema.json) — canonical event shape
- [cli.md](contracts/cli.md) — primary local access surface for v0
- [source-registry.seed.json](contracts/source-registry.seed.json) — approved v0 source set and source defaults
- [openapi.yaml](contracts/openapi.yaml) — post-v0 hosted API outline for Sift Pro, not the primary access path for v0

## Runbooks

- [siftd.md](runbooks/siftd.md) — single-node operator runbook for the hosted `siftd` service

## Research

- [2026-03-06-agent-native-news-landscape.md](research/2026-03-06-agent-native-news-landscape.md) — market and protocol context behind the design

## Root Docs

- [README.md](../README.md)
- [manifesto.md](../manifesto.md)
- [AGENTS.md](../AGENTS.md)
- [llms.txt](../llms.txt)
- [project.manifest.json](../project.manifest.json)
