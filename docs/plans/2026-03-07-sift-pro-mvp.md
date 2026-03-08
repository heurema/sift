# Sift Pro MVP

Дата: 2026-03-07
Статус: draft

## One-liner

Sift Pro is the hosted, autonomous event delivery layer for solo power users and agents, while Sift Free remains the local-first CLI product.

## Purpose

This document defines the first monetized slice after the v0 local-first foundation.

It does not replace the v0 boundary.

It adds a hosted premium service on top of the same event model, provenance rules, and rights policy already defined for the local product.

## Product decision

### Free

Sift Free remains:

- local CLI;
- local sync;
- local SQLite store and local `output/` projections;
- user-controlled scheduling;
- full local ownership of state.

### Paid

Sift Pro at `$5/mo` is:

- a hosted autonomous ingest and clustering service;
- a hosted Postgres-backed canonical event store;
- an authenticated read-only API over canonical event records;
- a WebSocket stream for low-latency event and digest updates;
- registration and sign-in delegated to Zitadel;
- a `30d` hosted history window;
- a higher refresh target than the default local setup.

## Why this split

The free tier must remain a real product, not a crippled teaser.

The paid tier should not sell "the same data over HTTP" if the user still has to keep their own machine online.

The value of the paid tier is:

- cloud autonomy;
- better freshness;
- always-on delivery for agents;
- a stable integration surface without self-hosting work.

## Ideal user

The first paid user is a solo power user who runs one or more personal agents, scripts, or automations and wants hosted event delivery without building and operating their own aggregator node.

## MVP scope

The first paid MVP includes:

1. autonomous cloud ingest on an operator-managed node;
2. reuse of the same approved source registry and rights policy as the local product;
3. reuse of the same canonical `Event` model and provenance rules;
4. authenticated read-only REST endpoints for event retrieval;
5. authenticated WebSocket delivery for low-latency updates;
6. `30d` retained event history;
7. refresh target of `<= 5m` for the hosted pipeline;
8. Postgres as the first hosted canonical store;
9. Zitadel-backed registration and user identity;
10. Zitadel-issued bearer auth for REST and WebSocket access;
11. single-node deployment as an acceptable first operating model.

## Explicit non-goals for the first paid MVP

Do not include these in the first paid release:

- premium content categories;
- team workspaces or RBAC;
- write APIs;
- user-defined source onboarding without review;
- webhook delivery;
- LLM-generated explain/brief surfaces in the base `$5` tier;
- multi-region deployment work;
- replacing the local CLI as the primary free surface.

`Webhooks` are intentionally deferred to `v1.1`.

They are useful, but not required to validate the hosted product.

## Service model

The paid service must be autonomous.

It cannot depend on a user's local CLI process being online.

That means:

- scheduling happens in the hosted environment;
- ingest happens in the hosted environment;
- clustering and scoring happen in the hosted environment;
- canonical state is stored in the hosted environment.

The local CLI remains valuable for:

- free usage;
- debugging and validation;
- local-first workflows;
- future fallback or hybrid modes.

## Delivery surfaces

### REST

The REST surface should extend the current remote API direction already outlined in [docs/contracts/openapi.yaml](../contracts/openapi.yaml).

The initial paid REST endpoints remain:

- `GET /v1/events`
- `GET /v1/events/{event_id}`
- `GET /v1/digests/{scope}/{window}`

### WebSocket

The paid MVP also adds:

- `GET /v1/ws` as the WebSocket upgrade entry point.

The initial stream contract should support at least:

- `event.upserted`
- `digest.updated`

The stream is for notification and incremental delivery, not for replacing the canonical REST read path.

Clients should always be able to re-fetch authoritative records over REST.

## Authentication

The first paid MVP uses Zitadel for registration and user identity.

Rules:

- registration and sign-in are delegated to Zitadel;
- auth is required for both REST and WebSocket access;
- the service accepts Zitadel-issued bearer tokens;
- the service validates issuer and audience against the configured Zitadel app;
- no local password store should exist inside Sift;
- no app-owned browser session layer is required in the first release;
- no fine-grained per-resource ACL in the first release.

## Data freshness and retention

The paid tier needs a real service-level differentiator.

For the first MVP, that differentiator is:

- hosted autonomous refresh target of `<= 5m`;
- `30d` retained history;
- availability independent of the user's laptop or workstation.

The first paid launch does not require a larger source catalog than v0.

Source expansion should happen only through the normal rights-review path.

## Initial technical architecture

The simplest acceptable first architecture is:

1. one Go server binary;
2. one scheduler inside the same process;
3. one hosted Postgres database inside the existing shared cluster;
4. one read-only HTTP API;
5. one WebSocket broadcaster fed from post-commit event changes.
6. one Zitadel OIDC integration for registration, sign-in, and token validation.

This is intentionally conservative.

The first paid MVP should optimize for speed of shipping and operational clarity, not for premature distributed systems work.

## Reuse rule

The hosted service should reuse the current Go pipeline packages wherever possible.

Do not fork the event model or create a separate "cloud-only" record shape.

The same canonical event layer should back:

- local CLI;
- local JSON and Markdown projections;
- hosted REST API;
- hosted WebSocket notifications;
- any later human UI.

## Sequence for implementation

1. extract and stabilize pipeline services that can run in both CLI and server modes;
2. add a Postgres-backed server binary with scheduler and read-only REST API;
3. add Zitadel integration and bearer token validation;
4. add hosted retention and operator runbooks;
5. add WebSocket delivery;
6. optionally add CLI remote mode after the hosted service is stable.

## Acceptance criteria

This plan is successful when all of the following are true:

1. the paid service produces fresh events without any user machine online;
2. the REST API serves the same canonical event model already used by local Sift;
3. the WebSocket stream delivers low-latency update notifications on top of that same model;
4. the service retains at least `30d` of queryable history;
5. the hosted service stays within the current rights policy and source registry rules;
6. the free tier remains fully usable as a standalone local-first product.

## Positioning

The product should be described simply:

`Sift Free` is the local-first event workspace.

`Sift Pro` is the hosted autonomous event service for agents.
