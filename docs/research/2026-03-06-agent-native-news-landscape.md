# Agent-Native News Landscape

Дата: 2026-03-06
Статус: note

## Summary

- Markdown is now a practical delivery format for AI agents, but it should not replace typed system contracts.
- `llms.txt` is emerging as a discovery layer for agent-readable project context.
- MCP is a credible future access layer, but not necessary for v0 if JSON, CLI, and Markdown are already sound.
- Crypto news is rich in feeds and aggregators, but still weak in agent-native event contracts and rights-aware output.
- The strongest differentiator is not "more summaries". It is event modeling plus provenance plus rights-aware delivery.

## Signals that matter

### Markdown as an agent delivery surface

Cloudflare publicly argues for `Accept: text/markdown` as a first-class consumption path for agents and shows large token savings versus HTML.

Implication:

- Markdown is a good output surface for reading and handoff;
- it is not sufficient as the canonical internal model.

### `llms.txt` as repo and site discovery

The `llms.txt` proposal gives agents a stable starting point:

- what the project is;
- where the main docs live;
- which deeper resources matter.

Implication:

- repos and products that target agents should expose a compact discovery file early.

### MCP is real, but timing matters

MCP has crossed from theory to product reality.

Implication:

- Sift should design its JSON contracts so that a future MCP adapter is straightforward;
- shipping MCP before the base event model stabilizes would be premature.
- for local agent workflows, a CLI is the more efficient first boundary.

### Crypto feeds are available, but rights are uneven

Official feeds and APIs exist across the crypto ecosystem, but source terms vary.

Implication:

- ingestion architecture must be rights-aware from day one;
- default to metadata plus summary until a source is clearly approved for more.

## Architectural conclusion

For this space, the right stack of abstractions is:

1. typed event records as canonical truth;
2. Markdown projections for reading;
3. CLI for local agent access;
4. `llms.txt` for discovery;
5. REST/JSON later when remote access is justified;
6. MCP later.

Anything that starts with page scraping or Markdown-only storage is likely to drift into a fragile media wrapper instead of an agent-native news layer.
