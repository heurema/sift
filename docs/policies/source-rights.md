# Source Rights Policy

## Purpose

News ingestion is useless if the rights model is sloppy.

This document defines the default behavior for how Sift collects, stores, and republishes information from external sources.

## Default posture

The safe default is:

- store metadata;
- store canonical URL;
- store source identifiers and timestamps;
- store generated summary derived from allowed inputs;
- store short excerpt only if clearly allowed or already exposed in an official feed.

Do not assume that public access means storage or republication rights.

## Allowed source classes

### Preferred

1. Official APIs with documented usage terms
2. Official RSS/Atom/JSON feeds
3. Official source pages where metadata-only collection is clearly safe

### Restricted

1. Unofficial scraping surfaces
2. Feeds or pages with unclear automation terms
3. Sources that reserve the right to disable or restrict machine access

Restricted sources require explicit review before implementation.

## Storage modes

### `metadata_only`

Store:

- title
- URL
- source
- timestamps
- author metadata if exposed
- tags/categories if exposed

Use when:

- rights are unclear;
- only indexing and linking are safe.

### `metadata_plus_excerpt`

Store:

- everything from `metadata_only`
- short excerpt from official feed or clearly allowed structured surface

Use when:

- excerpt publication is part of the feed contract or otherwise clearly permitted.

### `metadata_plus_summary`

Store:

- everything from `metadata_only`
- generated summary

Use when:

- internal analysis and derived summary are allowed by policy;
- full text is not stored.

### `full_text_allowed`

Store:

- article text or equivalent canonical body

Use only when:

- the source license or explicit terms allow it.

## Source profile requirements

Every source added to the registry must declare:

- `source_id`
- `source_name`
- `source_class` (`media` or `official`)
- `access_method`
- `url`
- `source_weight`
- `rights_mode`
- `excerpt_allowed`
- `summary_allowed`
- `default_editorial_type`
- `reviewed_at`
- `notes`

The canonical field list is enforced by the source loader (`DisallowUnknownFields`).
See `docs/contracts/source-registry.seed.json` for the reference shape.

### Naming boundary

Sources carry `rights_mode` to describe the ingestion permission level.
Events carry `rights.storage_mode` to describe the storage outcome after ingestion.
Both use the same enum values (`metadata_only`, `metadata_plus_excerpt`, `metadata_plus_summary`, `full_text_allowed`), but `rights_mode` is a source-level input and `storage_mode` is an event-level derived output.

## Output rules

- Human UI may link out freely to canonical sources.
- Human UI must not imply ownership of the original reporting.
- Agent-facing JSON and Markdown must preserve provenance.
- If a source is `metadata_only`, do not synthesize long quote-heavy output that reconstructs the article body.

## Operational rule

If rights are unclear, choose the narrower storage mode and record the uncertainty.
