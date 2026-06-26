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
| cfg schema | `internal/contracts/cfg` | service cfg model (incl. per-step `with:` args) + YAML parse + Validate against a Vocabulary (derived from docs/vocabulary) |
| interpreter | `internal/kernel/interpreter` | read cfg → route: Call (validate vs vocab → bus), Run (workflow stepping + branch on emitted fact), Successors (condition eval) |
| connectors | `internal/tools/connectors` | real sqlite tool on the bus: write/read with grant enforcement + crossing/mutation metering |
| ai | `internal/tools/ai` | infer tool on the bus behind a Backend interface (Codex driver deferred); cost fact at incur-time |
| MTG e2e | `test/e2e/mtg_test.go` | the worked example through the real substrate: trigger → interpreter → connectors write → fact → triggers (sequence inherited) → ai; one sequence_id across the cascade |
| storage | `internal/storage` | backend-configurable behind a `Dialect` (SQLite embedded **or** Postgres server), DSN-driven; forward-only per-dialect migrator; `?`→`$N` rebind. Postgres execution pending verification (testcontainers/Supabase) |
| spine | `internal/storage/facts` | ft_sequence/ft_run/ft_session with append-only triggers + access layer; causal-order query |
| daemon | `cmd/wuxing` | boots, loads boot manifest, inits bus, clean shutdown |
| cli | `cmd/wxg` | cobra command tree (library subcommands are stubs) |

The kernel's seven faces are all implemented (bus, lineage, scheduler, sessions,
triggers, launcher logic, interpreter). Stubs still `doc.go`-only:
`tools/{library,graph,processors}`, `contracts/{bus,sdk}`, `sdk`,
`storage/{dims,artifacts}`.

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

The kernel critical path (vocabulary → cfg grammar → contracts → interpreter) is
complete. Remaining work is wiring + content:

**M1 substrate acceptance is reached**: the MTG worked example runs end to end
through the real Go components (`test/e2e/mtg_test.go`). What remains is the
real-world I/O glue and content:

1. **integration glue** — real Docker `Engine` behind the launcher; triggers'
   cron clock + the bus subscription that feeds `OnEvent`; a real Codex `ai`
   backend; a real connector `Meter` writing the crossing/mutation fact tables.
2. **Postgres backend** — the storage `Dialect` abstraction + Postgres migrations
   are in place (DSN-driven, so Supabase/local Postgres is just `.env`). Next:
   verify the Postgres path (testcontainers in CI's integration job, or against a
   real Supabase DSN), wire pgx simple-protocol for the multi-statement
   migrations, port the connector to Postgres, and open the fact store (WAL for
   sqlite) on daemon boot so DBeaver/Tableau can read it live.
3. **storage detail/admin tables** — connector crossing/mutation + ai-call detail
   facts; the service index/manifest admin tables.
4. **content** — first-party service cfgs (messenger, state) and the thin
   `library` tool. (Per-step `with:` args are done — cfgs are self-driving: the
   interpreter merges a step's args with the accumulated facts into its payload.)

Deferred integration glue: the real Docker `Engine` (github.com/docker/docker)
behind the launcher interface; triggers' cron *clock* (robfig/cron driving
FireExternal) and the bus subscription that feeds `OnEvent`. The logic for all of
these is done and unit-tested; only the I/O wiring remains.

The worked example to aim for (acceptance): the MTG new-set notifier end-to-end
(checker → connector write → event trigger → notifier → messenger) observed in
the tracking tables under one sequence_id.
