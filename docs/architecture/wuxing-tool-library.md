# wuxing — library tool

*The catalog: persist and serve service definitions. A registry, not a runtime.*

> **Status:** designed at the contract level (this conversation). Thin by nature — that's the boundaries working. Functions are **provisional**, finalized once the **core** (its main caller) firms up.

## Responsibility (the one job)

Store service definitions and serve them — to the operator (catalog management) and to the core (runtime consultation). It **persists and serves; it never interprets, launches, or evaluates.** One real capability — *store a definition, serve its parts* — with the reads being slices of "serve."

## Surface — functions (provisional)

**Writes (lifecycle):**
- `register` — snapshot the **canonical** (hard-save of cfg + files) and create the index entry (draft → live), including building the image (the library *triggers* the build; the engine performs it).
- `deregister` — remove the entry. The **core** performs the drain check (no running instances); the library just removes.

**Reads (serve):**
- `get_definition` — return what the core needs to launch (image ref, envelope, allowlist, outputs).
- `get_successors` / `get_triggers` — return the stored successor / external-trigger **declarations** (return, never evaluate).
- `diff` — compare the live folder against the canonical snapshot (drift).
- `list` — enumerate the catalog.
- `read_declarations` — serve the structured declared tool-calls / declared script-functions metadata.

## Commands (a CLI mapping over a subset of the functions)

The operator surface is **commands**, not functions — `index`, `deindex`, `status`, and two cfg-introspection reads (`calls`, `functions`, e.g. `wxg library mtg calls 4`). `status` = **drift: live folder vs canonical snapshot** (subsumes the integrity check, built on the hard-saved canonical). Commands are ergonomic front-ends; the core calls the underlying functions directly with no command.

## Inputs & outputs (incl. emitted metadata)

In: a service's cfg + files (on register); a service id or event (on read). Out: definitions, declaration lists, drift reports, catalog listings. Like any tool, each invocation emits a **tool-call envelope** row (low volume — the library is the least-active tool). Its own facts/dimensions are minimal.

## Config surface

The library is configured by *the service cfgs it stores*, not by a cfg section of its own at runtime. The relevant cfg content it persists: identity, envelope, allowlist, outputs, **successor declarations** (each: successor + firing condition), external triggers, and the service's declared calls/functions (so introspection serves structure, never parses code).

## Boundaries (what it does *not* do)

Never **parses/interprets** cfg (the core does), never **launches** (core), never **evaluates** successor conditions (core), never **builds** the image itself (the engine; library triggers it), never performs the **drain check** (core's running registry). Every one of these absences is a boundary with the core or the engine.

## Callers

The **operator** (via CLI commands — index/deindex/status/calls/functions) and the **core** (runtime — get_definition / get_successors / get_triggers). Effectively **never** a service's cfg as a workflow step — the library adds almost nothing to the cfg *vocabulary*; it's defined by who consults it.

## Open / deferred

Final function shapes wait on the core's launch/parse flow (e.g. whether `get_definition` splits by consumer; whether the two resolves unify). Per-tool metadata tables follow the data-model deferral.
