# Design: Sift Pro Slice 1 Blueprint — Shared Runtime Extraction

DATE: 2026-03-07
STATUS: draft
SOURCES: current `cmd/sift` code scan, Sift Pro MVP plan, Sift Pro execution plan, local repository structure

## Purpose

This document turns `Slice 1` from a product idea into a concrete
implementation blueprint.

The goal of `Slice 1` is to extract the current local sync and digest runtime
logic from CLI command files into reusable internal packages, so that the same
pipeline can later power:

- the local CLI;
- the future hosted server binary;
- scheduler-driven background execution.

This slice is deliberately not the server.

It is the code-organization move that makes the server practical.

## Why Slice 1 Exists

Today the repository already has working runtime logic, but it is concentrated
inside CLI entry files.

That is acceptable for a local-first v0 CLI, but it becomes a problem for the
hosted path because:

- `runSync` mixes flag parsing with real pipeline orchestration;
- digest projection and atomic publication live in `cmd/sift/digest.go`;
- store open/migrate logic lives in `cmd/sift/main.go`;
- pipeline-specific helpers like run IDs and sync scope are defined in CLI code.

If the server is added on top of this shape directly, it will either duplicate
logic or start importing CLI-oriented files for runtime behavior.

Both outcomes are wrong.

## Current Runtime Flow

The current `sift sync` path is effectively:

1. parse flags in `cmd/sift/main.go`;
2. open and migrate SQLite via `openStore`;
3. load the source registry from disk;
4. persist the source registry snapshot;
5. fetch feed items per source;
6. normalize article candidates into canonical article records;
7. upsert articles into SQLite;
8. read articles back for clustering;
9. build event records;
10. replace the full event set in SQLite;
11. read events again for default digest generation;
12. publish digest projections to `output/`;
13. compute degraded source count;
14. create a run ID and persist the run summary;
15. render CLI output.

This is already a valid runtime.

The problem is where it lives, not whether it exists.

## Current Mixed Concerns

The main mixed-concern locations are:

### `cmd/sift/main.go`

This file currently owns:

- CLI routing and usage text;
- flag parsing for `sync`;
- SQLite open and migrate helper;
- fetch, normalize, cluster, and persist orchestration;
- run summary construction;
- digest publication trigger;
- output formatting.

That is too much responsibility for a CLI entry file.

### `cmd/sift/digest.go`

This file currently owns:

- digest CLI parsing;
- digest event selection rules;
- digest envelope construction;
- digest markdown rendering;
- atomic file publication helpers;
- default digest publication used by `sync`.

The `sync` pipeline already depends on this file indirectly, which is a signal
that digest logic is runtime code, not just CLI code.

### `cmd/sift` package overall

The package currently contains both:

- command-line interface concerns;
- reusable application behavior that should belong to internal packages.

That is the exact boundary `Slice 1` should correct.

## Slice Boundary

`Slice 1` should extract shared runtime behavior, but it should not expand the
product surface.

### In Scope

- reusable sync orchestration package;
- reusable digest projection package;
- reusable store-facing runtime boundary that can support local SQLite and
  hosted Postgres;
- shrinking CLI commands into parsing and rendering wrappers;
- tests for extracted runtime behavior and no-regression CLI coverage.

### Out of Scope

- server binary;
- HTTP API;
- WebSocket delivery;
- Zitadel auth integration;
- scheduler daemon mode;
- CLI remote mode;
- webhooks;
- the first hosted Postgres adapter itself.

If any of those appear in the diff, the slice is too large.

## Target Package and File Plan

The minimal credible extraction plan is:

### 1. Add `internal/pipeline/sync.go`

This package should become the shared orchestration home for the current sync
flow.

Responsibilities:

- define `Mode` for `full`, `fetch_only`, and `cluster_only`;
- define `Options` for runtime inputs such as registry path, output dir,
  current time source, and fetch function;
- define `Summary` as the runtime result returned to callers;
- run source loading, feed fetch, article normalization, event rebuild, digest
  publication, degraded source counting, and run persistence.

This package should own what is currently runtime behavior in `runSync`.

### 2. Add `internal/digest/projection.go`

This package should become the shared home for digest logic currently sitting in
`cmd/sift/digest.go`.

Responsibilities:

- define digest envelope and projection types;
- parse and validate digest windows;
- select digest events by scope and time window;
- build JSON and Markdown projections;
- publish one projection atomically;
- publish default digest projections for sync-time refresh.

The CLI command should call this package. The future server should call this
package too.

### 3. Add a shared store boundary and keep local SQLite bootstrap isolated

The current `openStore` helper should move out of `cmd/sift/main.go`, but
`Slice 1` should not hard-code the future server around SQLite.

Minimal shape:

- keep `sqlite.Open(...)` as the local low-level constructor;
- keep local open-and-migrate behavior available for the CLI;
- make shared runtime packages consume store interfaces rather than a concrete
  SQLite-first bootstrap path.

That gives `cmd/sift` a clean local path and leaves the hosted server free to
plug in Postgres next.

### 4. Shrink `cmd/sift/main.go`

After extraction, `cmd/sift/main.go` should own:

- command dispatch;
- sync flag parsing;
- sync argument validation;
- calling shared runtime code;
- rendering text or JSON output;
- usage text and exit code mapping.

It should no longer own the sync pipeline itself.

### 5. Shrink `cmd/sift/digest.go`

After extraction, `cmd/sift/digest.go` should own:

- digest CLI parsing;
- digest command argument validation;
- loading events from the store;
- calling shared digest projection functions;
- rendering JSON, Markdown, or text output.

It should no longer own digest selection rules or file publication internals.

### 6. Add runtime-focused tests

At minimum:

- `internal/pipeline/sync_test.go`
- `internal/digest/projection_test.go`

The current CLI tests should stay, but the core behavior must be tested below
the command layer after extraction.

## Recommended API Shape

The blueprint should stay pragmatic and avoid premature abstractions.

### `internal/pipeline`

Recommended shape:

- `type Mode string`
- `type Options struct { RegistryPath string; OutputDir string; PublishDefaultDigests bool; Now func() time.Time; FetchFeedItems func(context.Context, source.Source) ([]ingest.FeedItem, error) }`
- `type Summary struct { ... }`
- `type Store interface { ...current sync store methods... }`
- `func RunSync(ctx context.Context, store Store, opts Options) (Summary, error)`

Notes:

- use a function field for feed fetching instead of inventing a heavy client
  abstraction in this slice;
- inject `Now` for deterministic tests;
- define the store interface where it is consumed;
- do not make shared runtime depend on SQLite bootstrap details, because the
  hosted server will use Postgres.

### `internal/digest`

Recommended shape:

- `type Envelope struct { ... }`
- `type Projection struct { ... }`
- `func BuildProjection(records []event.Record, outputDir, scope, window string, generatedAt time.Time) (Projection, error)`
- `func PublishProjection(projection Projection) error`
- `func PublishDefault(records []event.Record, outputDir string, generatedAt time.Time) error`

The package should own the projection rules completely.

## File-Level Change Map

The expected file impact for this slice is:

1. [cmd/sift/main.go](/Users/vi/personal/sift/cmd/sift/main.go)
   Remove pipeline orchestration from `runSync` and delegate to
   `internal/pipeline`.

2. [cmd/sift/digest.go](/Users/vi/personal/sift/cmd/sift/digest.go)
   Remove digest core logic and delegate to `internal/digest`.

3. [internal/sqlite/store.go](/Users/vi/personal/sift/internal/sqlite/store.go)
   Keep the local store implementation, but move CLI bootstrap out of
   `cmd/sift` and avoid making it the shared runtime contract.

4. `internal/pipeline/sync.go`
   New shared sync runtime.

5. `internal/digest/projection.go`
   New shared digest runtime.

6. `internal/pipeline/sync_test.go`
   New runtime tests.

7. `internal/digest/projection_test.go`
   New digest runtime tests.

This is enough for `Slice 1`.

Anything materially beyond this is probably `Slice 2`.

## Implementation Order

The recommended order is:

1. move digest projection logic into `internal/digest` without changing CLI
   behavior;
2. introduce store-facing runtime interfaces before any hosted DB work;
3. move local SQLite bootstrap out of `cmd/sift`;
4. refactor `runSync` into a thin wrapper;
5. add runtime tests around extracted orchestration;
6. re-run existing CLI tests to ensure command behavior did not regress.

This order keeps the risk bounded because digest extraction is the smallest
runtime move and gives a reusable dependency for sync extraction.

## Validation Plan

This slice should be considered complete only if all of the following pass:

- `go test ./...`
- existing sync command behavior remains unchanged for `full`,
  `--fetch-only`, and `--cluster-only`
- default digest projection refresh still happens in sync paths that already do
  it today
- `sift digest` still publishes files atomically
- no CLI command imports from a future server package

The important rule is simple:

this slice may move code, but it must not change product behavior.

## Main Risks

### Risk 1: Hidden behavior drift

Moving code can silently change run summaries, timestamps, or projection paths
even when tests still compile.

Guardrail:

preserve existing command-level tests and add runtime-level assertions around
summary fields and generated file paths.

### Risk 2: Over-abstraction

If this slice introduces generic repository layers or speculative interfaces for
future backends, it will slow implementation without solving a real v1 problem.

Guardrail:

abstract only the behavior needed to run the existing pipeline outside the CLI.

### Risk 3: Package cycles

`cmd/sift` currently reaches into multiple internal packages directly. A rushed
extraction can create dependency cycles between `pipeline`, `digest`, and
`sqlite`.

Guardrail:

- `pipeline` may depend on `digest`, `source`, `article`, `event`, and
  `sqlite`-defined method sets;
- `digest` may depend on `event`;
- `sqlite` must not depend on `pipeline` or `digest`.

### Risk 4: SQLite leakage into hosted design

If the shared runtime extraction still assumes SQLite-specific bootstrap and
transaction posture, the later Postgres slice will either fork the runtime or
undo this work.

Guardrail:

keep SQLite as the local implementation, not as the shared runtime contract.

### Risk 5: Output coupling

Digest publication currently assumes a local `output/` layout.

That is still acceptable in `Slice 1`, but the runtime should take `outputDir`
as an explicit option instead of hardcoding CLI assumptions.

## Exit Criteria

`Slice 1` is done when:

1. the CLI still behaves the same from a user perspective;
2. sync orchestration no longer lives directly in `cmd/sift/main.go`;
3. digest projection logic no longer lives directly in `cmd/sift/digest.go`;
4. local SQLite bootstrap logic is shared outside the CLI package without
   becoming the hosted runtime contract;
5. the extracted runtime can be called by a future server binary without
   importing CLI command files.

## What Comes Immediately After

If this slice lands cleanly, the next slice is obvious:

- add `cmd/siftd`;
- wire long-lived config and scheduler loop;
- add a Postgres-backed hosted store;
- expose read-only REST endpoints over the shared runtime and hosted store;
- add Zitadel auth before shipping any hosted surface.

That is exactly why this slice exists.
