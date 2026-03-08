# Design: Sift Pro Reference Scenario — Hosted Event Service

DATE: 2026-03-07
STATUS: draft
SOURCES: Sift Pro MVP draft, Sift v0 execution spec, current OpenAPI outline, Concord reference-scenario style

## Purpose

This document defines the first narrow implementation path for Sift Pro.

The goal is not to design the final cloud platform. The goal is to force one
complete end-to-end path through the hosted product:

`autonomous hosted sync -> canonical store update -> authenticated REST read -> authenticated WebSocket notification`

The scenario must be narrow enough to ship quickly, but strong enough to prove
that Sift can become a real paid service instead of just a local CLI with a
thin HTTP wrapper.

## Scenario Choice

The reference implementation is a single-node hosted service for the existing
crypto event layer.

The initial service shape is:

- one operator-managed host;
- one Go server process;
- one in-process scheduler;
- one Postgres database in the existing shared cluster;
- one authenticated REST API;
- one authenticated WebSocket endpoint;
- one Zitadel-backed identity layer for registration and sign-in;
- one event category: `crypto`;
- one retention policy: `30d`.

This scenario is intentionally plain. The first paid slice does not need
multi-region, multi-tenant complexity, or a full SaaS control plane.

## Why This Scenario

This scenario was chosen because it proves the actual monetizable shift while
keeping the architecture disciplined.

It gives Sift Pro:

- true cloud autonomy, independent from any user laptop;
- a concrete premium surface for agents;
- reuse of the current event model and pipeline code;
- a narrow operational footprint that can be debugged quickly.

It also avoids the four easiest ways to overbuild the first paid release:

- team workspaces and RBAC;
- write APIs;
- webhook fan-out;
- a custom in-house identity system before Zitadel integration is proven.

If Sift cannot make this narrow hosted path reliable, it is not ready for a
larger cloud roadmap.

## Working Assumptions

The reference scenario assumes:

- the current Go pipeline remains the canonical implementation of fetch,
  normalize, cluster, score, and persist;
- the hosted service reuses the same source registry and rights policy as local
  Sift;
- the hosted service does not invent a second event model;
- one operator controls deployment, scheduler settings, and source health;
- the hosted service uses Postgres as its canonical store;
- Zitadel handles registration and user identity;
- Zitadel-issued bearer tokens are sufficient for the first release;
- the first paid user is a solo power user with one or more agents or scripts;
- the service is read-only for clients.

The scenario also assumes the local CLI remains supported and useful. The paid
service extends the system. It does not replace the local-first product.

## Service Boundary

The hosted service must be autonomous.

It cannot depend on a user's local `sift sync` process, local database, or
local `output/` directory.

For this scenario, the hosted service owns:

- source polling;
- article normalization;
- clustering and scoring;
- canonical persistence;
- digest materialization;
- authenticated delivery.

The local CLI remains responsible for:

- free local workflows;
- debugging and parity checks;
- operator fallback during hosted incidents;
- future hybrid or cache-assisted modes.

## Public Surface for the Reference Scenario

The first hosted surface is intentionally small.

### REST

The service exposes:

- `GET /v1/events`
- `GET /v1/events/{event_id}`
- `GET /v1/digests/{scope}/{window}`

These endpoints return the same canonical event and digest shapes already
defined for the local product.

### WebSocket

The service also exposes:

- `GET /v1/ws`

This endpoint upgrades authenticated clients to a low-latency update stream.

The stream is notification-oriented.

It does not replace REST as the canonical read path.

The first event types are:

- `event.upserted`
- `digest.updated`

Clients must treat stream payloads as advisory and re-fetch canonical records
over REST when they need full truth.

## Why REST Plus WebSocket

The premium layer is not selling "news pages." It is selling hosted delivery
for agents.

REST is the canonical retrieval path.

WebSocket is the low-latency delivery path.

This combination is enough to validate the core paid proposition:

- no local ingest process required;
- structured remote retrieval;
- near-real-time updates for agents;
- one stable surface for automations and scripts.

## Authentication Model

The first release uses Zitadel for registration and user identity.

Rules for the reference scenario:

- registration and login happen through Zitadel;
- every request to REST requires `Authorization: Bearer <token>`;
- WebSocket upgrade requires the same bearer token;
- the service validates issuer and audience against the configured Zitadel app;
- no local password store exists inside Sift;
- no app-owned browser-session database is required;
- no per-resource ACL is required;
- one paid user may later own multiple agents, but the first release does not
  require separate team membership modeling.

This is intentionally narrow. The first paid slice needs secure service access,
not a full identity platform of its own.

## Storage and Publication Rules

The hosted Postgres database is the authoritative store for the reference
scenario.

Derived outputs remain allowed, but they are not the primary truth boundary.

Rules:

- hosted Postgres is canonical;
- digest projections may still be materialized for operator inspection;
- REST responses must read from canonical stored state, not from Markdown;
- WebSocket notifications must only be emitted after successful commit of new
  canonical state;
- retention must delete or compact old data without breaking live reads.

## Freshness and Retention

The paid service needs a real service-level differentiator.

For this scenario, that differentiator is:

- autonomous refresh target of `<= 5m`;
- queryable hosted history for at least `30d`;
- service availability independent from a user's machine.

This does not require a larger source catalog in the first implementation
slice.

Source expansion remains a separate rights-reviewed decision.

## Execution Path

The narrow implementation path is:

### Slice 1: Shared Runtime Extraction

Extract reusable runtime services from the current CLI flow so that the same
pipeline can run in both command mode and long-lived server mode.

The output of this slice is not "an API." The output is a stable shared core
for:

- sync orchestration;
- event persistence;
- digest publication;
- store access from both CLI and server code paths.

### Slice 2: Hosted Server Skeleton

Add a dedicated server binary with:

- configuration loading;
- HTTP router;
- scheduler loop;
- health endpoints;
- operator startup path.

This slice proves that Sift can run as a continuously alive service rather than
as a one-shot CLI execution.

### Slice 3: Postgres Persistence Adapter

Add the first hosted persistence adapter on top of Postgres.

This slice should prove:

- canonical event persistence does not depend on SQLite;
- the hosted store still serves the same event model;
- writes, reads, and retention use one database of record;
- infra alignment with the existing shared Postgres cluster is real.

### Slice 4: Zitadel Authenticated REST Read Path

Expose the small read-only REST surface over the hosted canonical store.

This slice should prove:

- remote list and get behavior;
- parity with the local event model;
- bearer-token enforcement via Zitadel;
- clear operational failure modes.

### Slice 5: WebSocket Delivery

Add an in-process broadcaster for post-commit update notifications.

This slice should prove:

- authenticated connection upgrade;
- event notification after successful commit only;
- digest update notification after projection refresh;
- safe behavior for slow or disconnected clients.

### Slice 6: Retention and Operator Runbook

Add `30d` retention, operator controls, and recovery guidance.

This slice should prove the service can run as a paid offering, not just as a
demo binary.

## Expected Failure Paths

This scenario is useful because it creates clear failure modes that the
implementation must survive honestly.

### Failure 1: Fake Hosted Mode

The server exposes API endpoints but still depends on a local CLI or local cron
to populate state.

This fails the service boundary immediately. It is not a valid paid product.

### Failure 2: Divergent Event Shapes

The server path returns records that drift from the canonical local event model.

This fails the reuse rule and creates two incompatible Sift products.

### Failure 3: Pre-Commit Stream Emission

WebSocket notifications are emitted before canonical state is durably committed.

This creates phantom updates and breaks trust in the delivery surface.

### Failure 4: Retention Breaks Digests or Reads

Old records are removed in a way that breaks digest lookup, list pagination, or
single-event fetches for still-referenced items.

This turns retention into silent data corruption.

### Failure 5: Auth Drift

The service exposes protected endpoints but does not validate issuer, audience,
or token type consistently across REST and WebSocket.

This breaks the identity boundary and makes Zitadel integration cosmetic.

### Failure 6: Scope Explosion

The implementation drags in webhooks, teams, RBAC, billing UI, or
account UI before the narrow hosted service works.

This is not product depth. It is loss of implementation discipline.

## Exit Criteria

The reference scenario is complete when all of the following are true:

1. the hosted scheduler refreshes data without any user machine online;
2. `GET /v1/events` and `GET /v1/events/{event_id}` return canonical event
   records compatible with the current local model;
3. `GET /v1/digests/{scope}/{window}` returns hosted digest metadata over the
   same canonical event layer;
4. REST and WebSocket auth is enforced with Zitadel-issued bearer tokens;
5. `GET /v1/ws` delivers authenticated post-commit update notifications;
6. hosted canonical state lives in Postgres rather than SQLite;
7. the service retains at least `30d` of queryable hosted history;
8. the local CLI remains usable as a standalone local-first product.
9. REST and WebSocket surfaces stay coherent under repeated sync cycles.

## What This Scenario Does Not Prove

This scenario does not prove:

- full SaaS billing or account lifecycle;
- multi-user collaboration;
- webhook delivery guarantees;
- multi-region resilience;
- long-term storage architecture beyond the first Postgres shape;
- commercial success by itself.

That is intentional. The first hosted implementation should prove the service
boundary, the delivery boundary, and the monetizable convenience boundary.

## Implementation Principle

The service should be described simply:

`Sift Free` proves the event model locally.

`Sift Pro` proves the same event model can run as an autonomous hosted service.
