# Tool function vocabulary

The canonical signature catalogs for each tool. This is the **gating work** on
the critical path: the cfg grammar the interpreter parses is *derived from* these
signatures, so the interpreter cannot be finalized until the heavy tools (`ai`,
`connectors`) publish their catalogs here (Phase 1).

Each function entry specifies: **name · inputs (with types) · outputs (free text
vs schema) · allowlist semantics · resource envelope**.

| Tool | File | Status |
|---|---|---|
| ai | `ai.md` | Phase 1 — heavy, derives the grammar |
| connectors | `connectors.md` | Phase 1 — heaviest, derives the grammar |
| library | `library.md` | Near-final (registry; low cfg-vocabulary footprint) |
| graph | `graph.md` | Blocked on the interpreter ↔ graph boundary (Phase 6) |
| processors | `processors.md` | Minimal — follows the metadata schema |

Until a file exists here, the provisional starting point lives in the
[development plan](../../) (Phase 1 — provisional vocabulary tables) and in each
tool's `docs/architecture/wuxing-tool-*.md`.
