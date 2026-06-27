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
| ai agent mode | `internal/tools/ai/agent.go` | `AgentBackend` + `CLIAgent`: drive a real agent CLI (Hermes/Codex/Open Design) headlessly in a scratch dir; the `ai` "agent" op + `wuxing agent --brief` subcommand; config via `WUXING_AI_AGENT_*`. See [docs/quickstart-ai-agent.md](docs/quickstart-ai-agent.md) |
| library | `internal/tools/library` | in-memory Catalog: register/deregister + serve definition/successors/triggers/list/diff (core-consulted) |
| MTG e2e | `test/e2e/mtg_test.go` | the worked example through the real substrate: trigger → interpreter → connectors write → fact → triggers (sequence inherited) → ai; one sequence_id across the cascade |
| storage | `internal/storage` | backend-configurable behind a `Dialect` (SQLite embedded **or** Postgres server), DSN-driven; forward-only per-dialect migrator; `?`→`$N` rebind. Postgres execution pending verification (testcontainers/Supabase) |
| spine | `internal/storage/facts` | ft_sequence/ft_run/ft_session with append-only triggers + access layer; causal-order query |
| detail facts | `internal/storage/facts` | connector crossing/mutation + ai-call tables (migration 0003, both dialects) + `Detail` access layer keyed by the lineage stamp |
| admin index | `internal/storage/admin.go` | `wuxing_admin_service` table (migration 0004, both dialects) + Record/Get/List/Remove — declared service-index state (hard-saved cfg JSON) |
| metering | `internal/metering` | `StoreMeter` implements connectors.Meter + ai.Meter; tool facts persist to the detail tables, stamped from the envelope lineage |
| daemon | `cmd/wuxing` | boots, loads manifest, opens the fact store (`--store`, WAL), assembles the kernel via `kernel.Assemble`, logs readiness, clean shutdown |
| kernel assembly | `internal/kernel` | `Assemble` wires bus + scheduler + sessions + triggers + interpreter + library + StoreMeter over the fact store; `Close` tears down |
| run loop | `internal/kernel/runner.go` | `Kernel.Run` (spine + interpreter) + a `Runner` wiring onFire→scheduler.Submit and onAdmit→Run→fire successors. A cron trigger drives a full cascade (mtg→notifier), admission-gated, under one sequence_id. `Kernel.Register` wires a service's triggers |
| cli | `cmd/wxg` | cobra command tree (library subcommands are stubs) |

The kernel's seven faces are all implemented (bus, lineage, scheduler, sessions,
triggers, launcher logic, interpreter). Stubs still `doc.go`-only:
`tools/{graph,processors}`, `contracts/{bus,sdk}`, `sdk`,
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

Requires Go 1.25+ (per go.mod; installed at `C:\Program Files\Go`; may not be on every
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
3. **the run loop is driving cascades** — `Kernel.Run` + the `Runner` (onFire →
   Submit → onAdmit → Run → fire successors) run an admission-gated cascade
   in-process under one sequence_id; `Kernel.Register` wires a service's triggers.
   Sequence-closing and the admin service-index table are done. **The
   in-process system is complete and operational; the SQLite-verifiable work is
   exhausted.** The remaining items all need the user's infra:
   - **real Docker `Engine`** behind the launcher → run container-script services
     (start Docker Desktop);
   - **real Codex backend** for `ai` → real inference (Codex CLI + auth, or an
     OpenAI key in `.env`);
   - **Postgres execution verification** → a Supabase DSN in `.env` or Docker for
     testcontainers, plus pgx simple-protocol for the plpgsql migrations.

   Small follow-ups that pair with the above (not standalone-valuable yet): wire
   `library`/`Kernel.Register` to persist+load through the admin index; register
   the tools in the daemon (`cmd/wuxing`) with real backends so a service runs in
   the live daemon, not just the kernel test.
4. **admin tables** — the service-index/manifest admin tables.
5. **content** — first-party service cfgs (messenger, state). (Per-step `with:`
   args are done — cfgs are self-driving: the interpreter merges a step's args
   with the accumulated facts into its payload.)

Deferred integration glue: the real Docker `Engine` (github.com/docker/docker)
behind the launcher interface; triggers' cron *clock* (robfig/cron driving
FireExternal) and the bus subscription that feeds `OnEvent`. The logic for all of
these is done and unit-tested; only the I/O wiring remains.

The worked example to aim for (acceptance): the MTG new-set notifier end-to-end
(checker → connector write → event trigger → notifier → messenger) observed in
the tracking tables under one sequence_id.

## Handoff — resuming this project (read this if you're picking up cold)

**What's done:** the full kernel (seven faces) + the four tools (connectors, ai,
library real; graph/processors stubs) + the storage spine, detail facts, and
metering + the cfg interpreter + the MTG e2e (in `test/e2e`). The fact store is
backend-configurable (SQLite default / Postgres via DSN). The daemon
(`cmd/wuxing`) boots, opens the WAL fact store, and **assembles a live kernel**
(`kernel.Assemble`), then idles. Everything is unit/substrate-tested; **M1
(the worked example through the real Go components) is reached**.

**Where to pick up:** Next tasks item 3 — **the run loop**. The daemon assembles
the kernel but doesn't drive it yet. Wire `triggers.onFire` → open a spine
sequence/run/session (`facts.Spine`) → run the service's workflow via the
`interpreter` → close the run; wire `scheduler.onAdmit` → launch; register the
tools. The in-process (cfg-only) path is sqlite-verifiable now; the container
path needs the real Docker engine (below).

**Parked — needs the user's infrastructure (do NOT blind-debug via CI):**
- Postgres *execution* verification — needs Docker (testcontainers) running or a
  live Supabase DSN in `.env`. Also wire pgx simple-protocol for the
  multi-statement/plpgsql migrations when verifying.
- Real Docker `Engine` behind the launcher (run actual containers).
- Real Codex backend for the `ai` tool (real inference; needs Codex CLI + auth).
When you hit these, surface them to the user rather than guessing.

**How work flows (the loop convention):** build each feature on a pushed
`feat/*` branch → PR into `dev-claude` → CI green → self-merge, **keep the
branch**. Never delete branches, never push to `dev`/`main` directly. Open
`dev-claude → dev` promotion PRs for the user to validate; the user merges all
`dev`/`main` PRs. Commits are atomic + green; conventional types. Keep THIS file
current each iteration (status table + Next tasks) — it is the primary handoff.
