# CLAUDE.md — working context for wuxing

Persistent context for AI sessions on this repo: what wuxing is, what exists now,
how to work on it, and what's next. Keep this current as the project moves.

## What wuxing is

A personal control plane — one surface to set up, observe, control, and
orchestrate a whole fleet of services. Mechanically a **declarative
cfg-interpreter**: the config is the program, the runtime interprets it and
routes calls to tools and services over an in-process bus. AI is a first-class
but *governed* capability (a bounded content producer in a sealed sandbox).

Three tiers:
- **wuxing (kernel)** — the cfg-interpreter + router; a long-running compiled Go
  daemon. Its seven faces: interpreter, bus, scheduler, sessions/registry,
  triggers, launcher, lineage.
- **tools** — the platform verbs, compiled into the daemon, named in the boot
  manifest: `library`, `graph`, `processors`, `connectors`, `ai`. Tools compose,
  routed over the bus.
- **services** — sealed user boxes, run as Docker containers, listed in the
  library index. Mutually ignorant; the only thing crossing a service boundary is
  an event on the bus.

Full design lives in [`docs/architecture/`](docs/architecture/); the tool
function vocabulary (the gating work the interpreter waits on) will live in
[`docs/vocabulary/`](docs/vocabulary/).

## Status — what exists

Built and tested (Go, pure-Go deps, no cgo in app code):

| Area | Package | What |
| --- | --- | --- |
| lineage | `internal/kernel/lineage` | stamp (sequence/run/session/call/tool_function ids + order fields) + UUID minter |
| bus | `internal/kernel/bus` | message envelope (call/return/event) + in-process bus: routed request/reply, non-blocking pub/sub (drop-and-count), race-clean |
| scheduler | `internal/kernel/scheduler` | dual-resource admission (memory + AI window), never-partial, priority+FCFS, backfill, patience (escalate/fail), overclock reserve band |
| sessions | `internal/kernel/sessions` | running registry of live instances; the drain check (HasRunning) the library's deregister consults |
| triggers | `internal/kernel/triggers` | cron + event rules; sequence propagation (external opens a new sequence, event inherits the cause's) |
| launcher | `internal/kernel/launcher` | Engine interface + launch logic: provision/sweep scratch, inject env, apply mem limit, track instances (Docker driver deferred) |
| cfg schema | `internal/contracts/cfg` | service cfg model + YAML parse + Validate against a Vocabulary (derived from docs/vocabulary) |
| storage | `internal/storage` | pure-Go sqlite open + forward-only embedded migrator (schema_migrations) |
| spine | `internal/storage/facts` | ft_sequence/ft_run/ft_session with append-only triggers + access layer; causal-order query |
| daemon | `cmd/wuxing` | boots, loads boot manifest, inits bus, clean shutdown |
| cli | `cmd/wxg` | cobra command tree (library subcommands are stubs) |

Stubs still `doc.go`-only: `kernel/interpreter`,
`tools/{library,graph,processors,connectors,ai}`, `contracts/{bus,sdk}`,
`sdk`, `storage/{dims,artifacts}`.

## Branch & PR workflow (IMPORTANT)

- `main` — stable, the user's gate. Never pushed to directly.
- `dev` — integration gate; the user validates and merges **all** PRs into it.
- `dev-claude` — the agent's integration branch; the agent self-merges feature
  PRs here, then opens `dev-claude → dev` promotion PRs for the user.
- Every feature: its own **pushed** `feat/*` branch → PR → merge. **Do not delete
  branches on merge** (the user wants them browsable). **Do not** merge locally
  without pushing the branch first.
- Commits: atomic and green (a change and its tests together). Conventional
  types: `feat`/`fix`/`docs`/`refactor`/`test`/`chore`/`ci`. No "phase N" language.

## How to build, test, lint

Requires Go 1.23+ (installed at `C:\Program Files\Go`; may not be on every
shell's PATH — prepend it). `golangci-lint` v2 (pinned v2.12.2 in CI).

```sh
go build ./...
go test ./... -short            # -race needs a C compiler; CI runs it, local here does not
golangci-lint run               # v2 schema; formatters gofumpt + goimports
```

CI (`.github/workflows/`): `sanity.yml` (lint+unit+build on every feature-branch
push), `ci.yml` (full 3-OS matrix + integration + commit-lint + 70% coverage
floor on PRs and dev/main), `release.yml`, `codeql.yml`.

## Next tasks

In rough dependency order (kernel faces + the tool vocabulary are done):

1. **interpreter** (`kernel/interpreter`) — parse a cfg, validate each
   tool.operation against the vocabulary, route the call over the bus, feed the
   return forward, evaluate successor conditions on the emitted fact.
2. **first-party services** (messenger, state) and the **MTG e2e**.

The cfg schema (`contracts/cfg`) is done; the bus envelope already lives in
`kernel/bus`; the SDK contract (`contracts/sdk`) is a thin follow-up. The tool
vocabulary is drafted in `docs/vocabulary/` (ai, connectors, library);
graph/processors stay minimal/blocked per the design.

Deferred integration glue: the real Docker `Engine` (github.com/docker/docker)
behind the launcher interface; triggers' cron *clock* (robfig/cron driving
FireExternal) and the bus subscription that feeds `OnEvent`. The logic for all of
these is done and unit-tested; only the I/O wiring remains.

The worked example to aim for (acceptance): the MTG new-set notifier end-to-end
(checker → connector write → event trigger → notifier → messenger) observed in
the tracking tables under one sequence_id.
