# Architecture

The canonical design documents for wuxing. Start with the concept doc, then the
core, then the per-tool and data-model docs.

| Doc | What it covers |
|---|---|
| [wuxing-concept.md](wuxing-concept.md) | The whole system: three tiers, core principles, the AI/agent layer, scheduler, observability, the worked example |
| [wuxing-core.md](wuxing-core.md) | The kernel and its seven faces (interpreter, bus, scheduler, sessions, triggers, launcher, lineage) |
| [wuxing-data-model.md](wuxing-data-model.md) | The platform's own record: the lineage spine, the four storage tiers, naming, the tool-call envelope |
| [wuxing-tool-ai.md](wuxing-tool-ai.md) | The `ai` tool — infer + bounded agent sessions, governance |
| [wuxing-tool-connectors.md](wuxing-tool-connectors.md) | The `connectors` tool — data I/O + the data-plane meter |
| [wuxing-tool-graph.md](wuxing-tool-graph.md) | The `graph` tool — intra-service workflow orchestration |
| [wuxing-tool-library.md](wuxing-tool-library.md) | The `library` tool — the service catalog |
| [wuxing-tool-processors.md](wuxing-tool-processors.md) | The `processors` tool — derive metadata/metrics |

## Provenance

These are committed copies of the authoring drafts kept in the top-level
`context/` directory (the author's working scratch, not committed). When a design
doc changes, update the draft in `context/` and re-copy into `docs/architecture/`
in the same PR so the committed tree never drifts. The boundary-resolution notes
(e.g. the interpreter ↔ graph split, settled in Phase 6) are added here as new
files alongside the originals.

## Vocabulary

The tool function catalogs — the gating work the interpreter waits on — are
derived in Phase 1 and live in [`../vocabulary/`](../vocabulary/).
