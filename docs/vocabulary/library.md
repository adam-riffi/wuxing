# library — function vocabulary

The catalog: persist and serve service definitions. A registry, not a runtime —
it **persists and serves; it never interprets, launches, or evaluates**. Low
addition to the cfg vocabulary; defined by who consults it (the operator and the
core). Each call emits a tool-call envelope; the library is the least-active tool.

## Writes (lifecycle)

| Function | Inputs | Output |
|---|---|---|
| `register` | service cfg + files | index entry id (draft → live) |
| `deregister` | service id | ack |

- **`register`** — snapshot the **canonical** (hard-save of cfg + files), create
  the index entry, and trigger the image build (the engine performs it).
- **`deregister`** — remove the entry. The **core** performs the drain check (no
  running instances, via `sessions.HasRunning`); the library just removes.

## Reads (serve)

| Function | Inputs | Output |
|---|---|---|
| `get_definition` | service id | image ref, envelope, allowlist, outputs |
| `get_successors` | service id | successor declarations (return, never evaluate) |
| `get_triggers` | service id | external-trigger declarations |
| `diff` | service id | drift: live folder vs canonical snapshot |
| `list` | filters | catalog listing |
| `read_declarations` | service id | structured declared calls / script-functions |

The reads are slices of "serve." `get_successors`/`get_triggers` **return**
declarations; the core evaluates them. `diff` (a.k.a. `status`) is the drift /
integrity check built on the hard-saved canonical.

## CLI mapping

The operator surface is commands over a subset: `wxg library index` (register),
`deindex` (deregister), `status` (diff), and the cfg-introspection reads `calls`
and `functions` (e.g. `wxg library mtg calls 4`). The core calls the underlying
functions directly, with no command.

## Boundaries

Never parses/interprets cfg (the core), never launches (core), never evaluates
successor conditions (core), never builds the image itself (the engine; library
triggers it), never performs the drain check (the core's running registry).

## Open

Final function shapes wait on the core's launch/parse flow (e.g. whether
`get_definition` splits by consumer). Per-tool metadata tables follow the
data-model deferral.
