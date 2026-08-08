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
| cfg schema | `internal/contracts/cfg` | service cfg model: envelope (request/limit/ai_request/priority/max_wait/on_starve), allow, triggers, workflow (tool XOR `script:` steps, `with:`, branch/next), successors; YAML parse + Validate; `LoadDir` (a service is a folder). See [docs/cfg-guide.md](docs/cfg-guide.md) + [docs/cfg-audit.md](docs/cfg-audit.md) |
| services on disk | `services/` | the worked example as real cfgs (`mtg`, `notifier`), loaded by the daemon via `--services` and persisted to the admin index |
| interpreter | `internal/kernel/interpreter` | read cfg → route: Call (validate vs vocab → bus), Run (workflow stepping + branch on emitted fact), Successors (condition eval) |
| connectors | `internal/tools/connectors` | real sqlite tool on the bus: write/read with grant enforcement + crossing/mutation metering |
| ai | `internal/tools/ai` | infer tool on the bus behind a Backend interface (Codex driver deferred); cost fact at incur-time |
| ai agent mode | `internal/tools/ai/agent.go` | `AgentBackend` + `CLIAgent`: drive a real agent CLI (Hermes/Codex/Open Design) headlessly in a scratch dir; the `ai` "agent" op + `wuxing agent --brief` subcommand; config via `WUXING_AI_AGENT_*`. See [docs/quickstart-ai-agent.md](docs/quickstart-ai-agent.md) |
| library | `internal/tools/library` | in-memory Catalog: register/deregister + serve definition/successors/triggers/list/diff (core-consulted) |
| processors | `internal/tools/processors` | the derive tool: rolls raw facts into `processors_ft_run` / `processors_ft_sequence` (migration 0005, both dialects); `SummarizeRun`/`SummarizeSequence` wired into the run loop (auto-derived on close). Not a cfg verb — platform machinery |
| MTG e2e | `test/e2e/mtg_test.go` | the worked example through the real substrate: trigger → interpreter → connectors write → fact → triggers (sequence inherited) → ai; one sequence_id across the cascade |
| storage | `internal/storage` | backend-configurable behind a `Dialect` (SQLite embedded **or** Postgres server), DSN-driven; forward-only per-dialect migrator; `?`→`$N` rebind. Postgres execution pending verification (testcontainers/Supabase) |
| spine | `internal/storage/facts` | ft_sequence/ft_run/ft_session with append-only triggers + access layer; causal-order query |
| detail facts | `internal/storage/facts` | connector crossing/mutation + ai-call tables (migration 0003, both dialects) + `Detail` access layer keyed by the lineage stamp |
| admin index | `internal/storage/admin.go` | `wuxing_admin_service` table (migration 0004, both dialects) + Record/Get/List/Remove — declared service-index state (hard-saved cfg JSON) |
| metering | `internal/metering` | `StoreMeter` implements connectors.Meter + ai.Meter; tool facts persist to the detail tables, stamped from the envelope lineage |
| daemon | `cmd/wuxing` | boots, loads manifest + services (`--services`), opens the fact store (`--store`, WAL), assembles the kernel, **registers tool handlers** (ai over the auto-detected agent CLI; connectors over `domain.db` with the union of loaded grants), **starts the cron watcher**, serves the control API (`--api`), clean shutdown. **The core loop is closed**: cfg on disk → cron fires → admission → real AI → facts/rollups, zero Go |
| cron watcher | `internal/kernel/triggers/cron.go` | `StartCron` parses every registered rule (robfig/cron: 5-field, @descriptors, CRON_TZ) and fires due schedules via FireExternal (new sequence per fire); bad spec fails the start. cfg: trigger `timezone`, service-level `concurrency: allow\|forbid` (forbid = skip overlapping fire, enforced in Runner.skipOverlap) |
| control API | `internal/control` | loopback HTTP/JSON daemon channel: `GET /state` + `POST /run` + `GET /library` + `POST /library/index` + `POST /library/deindex`; wire types + `CatalogFuncs` live here |
| AI window | daemon + kernel | `Assemble(store, mem, aiWindow)`; the daemon refills the window hourly (clock-refilled rate lane). `Scheduler.Fits` refuses never-admittable envelopes at Register (loud, not a silent drop); Runner rolls back bookkeeping when Submit refuses a job |
| kernel assembly | `internal/kernel` | `Assemble` wires bus + scheduler + sessions + triggers + interpreter + library + StoreMeter over the fact store; `Close` tears down |
| run loop | `internal/kernel/runner.go` | `Kernel.Run` (spine + interpreter) + a `Runner` wiring onFire→scheduler.Submit and onAdmit→Run→fire successors. Jobs carry the FULL cfg envelope; starved-out jobs (ExpiryFail) release their sequence. `Kernel.Register`/`Kernel.Deregister` wire/unwire a service's triggers; drain check in Deregister |
| cli | `cmd/wxg` | cobra tree; `wxg infer detect/chat/agent`; `wxg runs`/`wxg show`; `wxg state`/`wxg run`; **`wxg library index/deindex`** (HTTP to daemon); **`wxg services`** (direct DB read — no daemon needed); status/calls/functions stubs |
| codex backend | `internal/tools/ai/codex.go` | `CodexCLI` implements **both** `Backend` (infer) and `AgentBackend` (agent) via `codex exec`; config `WUXING_CODEX_*` |
| agent auto-detect | `internal/tools/ai/detect.go` | scans PATH for known agent CLIs (codex, claude, gemini, hermes, od); `Detect`/`DetectDefault`/`ResolveAgent`; `CLIAgent` now implements `Backend` too. No manual wiring — used by `wxg infer` + `wuxing agent`. See [docs/commands.md](docs/commands.md), [docs/tui-build.md](docs/tui-build.md) |

The kernel's seven faces are all implemented (bus, lineage, scheduler, sessions,
triggers, launcher logic, interpreter). Of the five tools, four are real
(connectors, ai, library, processors); `tools/graph` is the last stub (its job —
intra-service workflow stepping — is largely done by the interpreter; the
graph-vs-interpreter boundary is the open question). Other stubs still
`doc.go`-only: `contracts/{bus,sdk}`, `sdk`, `storage/{dims,artifacts}`.

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

**THE CORE LOOP IS CLOSED (v1 core, verified live 2026-07-02):** a cfg on disk →
`wuxing --services` loads it → the cron watcher fires it → admission-gated run →
real AI (auto-detected codex) → facts + rollups + sequence close → visible in
`wxg runs`/`show`. Zero Go code, zero manual trigger.

Remaining, in order:

1. ~~**`POST /run` + `wxg run <service>`**~~ — **done** (verified live: fired the
   on-disk mtg service through real codex, 47s inference, success + facts).
   Still open from it: the run-control params (docs/cli-run-control.md:
   --step/--with/--sequence/--no-cascade/--dry-run).
2. ~~**`wxg library index/deindex` + `wxg services`**~~ — **done** (2026-08-07).
   `Kernel.Deregister` + `Triggers.UnregisterService`; control API catalog endpoints
   (`GET /library`, `POST /library/index`, `POST /library/deindex`); daemon wires
   closures + persists to/reloads from admin index; CLI `wxg library index/deindex`
   (HTTP to daemon) and `wxg services` (direct DB). v1 is feature-complete.
3. **run-control params on `/run`** (docs/cli-run-control.md) — `--step`/`--from`/
   `--to`/`--only` targeting; `--with`/`--input`; `--sequence`/`--no-cascade`/
   `--then`; `--dry-run`/`--wait`/`--detach`/`--priority`/`--force`. Extend
   `RunRequest` then surface in `wxg run`.
4. **real Docker `Engine`** behind the launcher (needs Docker Desktop) — executes
   `script:` steps per docs/cfg-guide.md's contract (stdin fact → stdout fact,
   exit → outcome); brings cfg params image/env/timeout/retry/on_error
   (docs/cfg-audit.md tier 3) and makes `wxg state` queues/sessions light up.
4. **Postgres execution verification** — Supabase DSN or testcontainers + pgx
   simple-protocol for the plpgsql migrations.
5. **content** — first-party service cfgs (messenger → Discord, state); make the
   shipped `services/mtg` real (Scryfall via script once Docker lands).
6. **ai governance block** — per-service model/backend, max_cost, max_turns;
   ai grant enforcement (audit tier 2).

Deferred integration glue: the real Docker `Engine` (github.com/docker/docker)
behind the launcher interface — the last big I/O seam. (The cron clock is done:
robfig/cron drives FireExternal via `StartCron`.)

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

**Where to pick up:** v1 is feature-complete. The catalog commands are done
(item 2 closed 2026-08-07). Next is the **run-control params on `/run`** (item 3
in Next tasks) — extend `control.RunRequest` with targeting/input/sequencing/
execution-mode fields (docs/cli-run-control.md) then surface via `wxg run` flags.
After that: the Docker engine (item 4).

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
