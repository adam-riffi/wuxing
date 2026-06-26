# wuxing

*A personal control plane — one surface over your whole fleet of services. At heart, a declarative cfg-interpreter; in spirit, infrastructure-first agency.*

> **Naming.** **wuxing** is the brain/interpreter itself (the kernel). It also names the system as a whole — kernel plus tools plus services. Intentional overload; context carries it, like "Linux."
>
> **Tiers:** **wuxing** (kernel/interpreter) loads **tools** (platform verbs — library, graph, processors, connectors, ai), and tools run **services** (sealed user boxes). The boot manifest lists tools; the library index lists services.

## What it is

A personal control plane: a single place to **set up**, **observe**, **control**, and **orchestrate** all your services — instead of juggling siloed, atomic services. Mechanically, a **declarative cfg-interpreter**: the config is the program, the runtime is its interpretation, routed to tools and services over a bus. Lineage: Terraform, Kubernetes, Airflow.

It also has an animating idea beyond plumbing: **a personal control plane where bounded, sandboxed agents produce governed content.** AI is a first-class capability — but governed as infrastructure: an agent is a content producer in a sealed room with supervised doors, never an ungoverned actor and never a system that rewrites itself. The bar is *useful*, not revolutionary.

## Three tiers

- **wuxing — the kernel.** The cfg-interpreter and router. The one irreducible, hardcoded part. A long-running daemon, **compiled** (Go is the canonical fit; choice open).
- **tools — the platform verbs.** wuxing's own code, in the daemon, in the boot manifest. Compiled, same language as the kernel. **Tools compose.**
- **services — sealed user boxes.** Runtime content, in the library index, run as containers. **Services are sealed** — mutually ignorant. Any language.
- **first-party services — the suite.** Curated services that *ship* with wuxing (messenger, state/datastore, webhook/ingress). Architecturally identical to user services; distinguished only by provenance. Not a new tier.

## Code vs content — the central line

The tool/service split *is* code-versus-content, and it decides what has a lifecycle.

- **Tools are code** — wuxing's own. Added by *writing*, removed by *erasing* and reworking the core. **No runtime tool deletion**; the manifest is **source-level**, edited in development. ("Modular interpreter" = a modular codebase, not a runtime plugin manager.)
- **Services are content** — created, indexed, and deleted through the platform at runtime. Lifecycle machinery (index, draft/live, delete) is **services-only**.

**Disabling a tool equals deleting it** once any service depends on it; only permanence differs. What a tool *concretely* is vs a service is **not** compiled-vs-interpreted (orthogonal) — it's **where the code lives and when it integrates**: a tool is part of the wuxing program, build-time, manifest-declared; a service is external cfg + scripts, dropped in, indexed, run as an isolated container. Both are cfg + scripts against the same SDK.

## Core principles

- **Declarative: config is the program.** A **one-shot interpreter** (read → interpret → route), not a reconciler. The config *format is the language*.
- **Brain barely decides; hands do.** wuxing's only verb is *read cfg and route*; work lives on the far side of the bus. The kernel is almost empty.
- **cfg + scripts is the universal anatomy.** Every tool and service is cfg + scripts against the SDK contract; wuxing dogfoods its own SDK.
- **Tools compose; services are sealed.** Composition stays **routed** through the bus, never by linking code. Dependency declared; call routed.
- **Services are islands; events are the only bridge.** No service connects to or calls another — the only thing crossing a service boundary is an **event** on the bus that may *trigger* another service. A workflow lives *inside* a service (routing across its own parts), never across services. So there is **no cross-service flow, no saga, no compensation** — things that never connect can't fail together. **No polymorphism:** a service is one concrete thing, fixed parts and shape.
- **Two orchestrators, one substrate.** **graph** drives *declared, intra-service* flows (DAGs, branch on a value); the **ai tool** drives *emergent* flows (model-driven agent loops). Different control logic, **same** scheduler, sessions, containers, and bus. The russian doll: the two engines nest.
- **Agents propose, wuxing disposes.** An agent's reasoning is *unprivileged* — every resource-consuming action (a tool call, a sub-session, memory) is a *request* to wuxing, admitted or queued like any job. Capability-based security: the agent owns nothing it didn't ask for.
- **Variation is a driver, never a service above the tool.** Backends (DB drivers, LLM/agent runtimes) sit behind a tool's stable interface; `type` in cfg selects.
- **Capture at the source, decide later.** Record full events; query after.
- **Data plane vs control plane / Shared schema is the contract.**

## The interpreter model

wuxing boots, reads its **boot manifest** (source-level), loads the tools named there, then reads declarations and routes them. **The irreducible kernel** is thin but not empty: the bus, queue, and session/concurrency/**scheduler** state can't live behind the bus. **Three registries:** **boot manifest** (tools) · **library index** (service definitions + lifecycle) · **running registry** (live instances). Definitions vs instances — image-vs-container.

## Tools

- **library — set up & catalog.** *Build* (scaffold a service from cfg via `wuxing new`) + *store* (service definitions — each carrying its own internal workflow). Holds the **index** gate: draft/manual → indexed/live. *(Name provisional.)*
- **graph — orchestration.** Runs a service's *internal* workflow — its DAG across its own parts — step by step, awaiting confirmations; branch nodes route on a step's output. A single call is a one-step DAG. (Inter-service coupling is event triggers, not graph.)
- **processors — derive.** Turn raw signals into metadata/metrics; build the **fact table** from connector crossings.
- **connectors — data I/O.** Move data across the boundary via a **driver per backend** + `.env` creds. Also the **data-plane meter**: each crossing emits a movement-fact (audit + volumetry). *Don't meter the meter.*
- **ai — infer & agency.** The AI verb. Two modes and a governed agent loop — full treatment below.

## Services

A service is a **folder**: a `services/` directory, one folder per service, each holding one or more yml cfgs plus its scripts (Python, shell, SQL, JS). The combination is the service; **the cfg is its ID card.** (Trajectory: today cfg + scripts; the goal is everything *in* the cfg as services become declarative enough to need no script.)

**Service = the whole capability; workflow = the route.** A service holds *all the parts for every path* its workflow might take; a run assembles the specific route through them. The MTG service isn't one step — it's checker, scraper, updater and their internal wiring.

**Self-contained to the point of duplication.** A service shares nothing with another — no cross-service libraries — so in the code-using version ten services needing the same script hold ten identical copies. Maximal isolation at the cost of DRY; the cfg-only goal eventually dissolves the duplication.

**Islands.** A service never calls another; on finish it emits an **event**, blind, and wuxing's triggers decide what (if anything) runs next. It does work and **reports a fact** (`new_set: true`), never hearing what comes after.

**External APIs are script-based, for now.** Reaching the long tail of bespoke APIs (JS SDKs, Python clients, raw HTTP) lives in the service's own script today — an explicit **interim** bending of the sandbox rule, since no language-agnostic alternative exists yet. The north star is a **universal connector** folding API reach into governed cfg. Connectors already handle the uniform backends (SQL, known sinks).

**Declarative vs scripted:** pure tool-composition can be **yml-only** (the generic runner routes it); a script is needed only for **bespoke logic**.

## Failure, state & idempotency

**Two-tier error handling.** wuxing's built-in layer recognizes generic error *classes* (crash, timeout, non-zero exit) and applies the **cfg-declared policy** — stop, retry (with backoff), or standby. The service's layer **classifies** domain-specific errors wuxing can't read (a DB error code, an API rate-limit) and *translates* them into that same fixed vocabulary. The service classifies; wuxing controls — the outcomes are wuxing's, the interpretation is the service's.

**No cross-service saga.** Because no workflow spans services, there is never a distributed transaction to undo. A service's workflow is atomic *to itself*; a late-stage consequence (notify fails after the write landed) is handled *inside* the service by its author, because all the parts are theirs. Partial failure is a within-service design decision, not a platform mechanism.

**Idempotency via a success-only high-water mark.** A service keeps a small persistent marker (`last set seen: Strixhaven`) it reads to decide what's new and **writes only on a successful run**. A failed or double-fired run doesn't advance it, so the next run reprocesses from the same point — no skip, no double-count. Update-on-success is what makes retries safe.

**Two stores per service.** That checkpoint is *distinct from the ephemeral scratch space*: the scratch space dies each run (an agent's churn), the checkpoint survives. A throwaway play area plus a tiny durable marker.

## cfg + scripts — the SDK

Drop a conforming cfg + script, index it. That contract *is* the SDK; tools obey it too. **Exception:** wuxing itself can be packaged as cfg + scripts but loads and runs the rest — the bootstrap can't be bootstrapped.

## Secrets — `.env`

Environment, loaded at boot, below everything (12-factor). The cfg holds the *reference* (`${SCRYFALL_KEY}`); `.env` holds the *value*; never committed. A tool *references* a secret; it never *stores* one.

## The AI / agent layer

The animating idea, and the part most distinct from the tools around it.

### Lineage

**Hermes Agent (NousResearch)** inspired the agent idea — an always-learning, skill-writing personal agent. But Hermes is **agent-first** (one assistant you converse with); wuxing is **infrastructure-first** (a control plane where AI is one tool). And wuxing's agent is deliberately *narrower* than Hermes': not a self-improving actor, but a **bounded content producer** (why, below).

### The two modes

- **infer** — one-shot completion (payload → result). Generic, domain-blind; prompt/context/model/schema in the caller's cfg.
- **agent session** — a model-driven, open-ended loop (think → act → observe → decide), for tasks whose next step isn't declarable up front.

graph orchestrates *declared* flows; the ai tool orchestrates *emergent* ones — but on the **same substrate** (same scheduler, sessions, containers, bus). Only the control logic differs. The russian doll: the two engines nest.

### The agent is a bounded content producer

Not an actor that operates the system — a worker that **produces content within a bounded space.** Its writing ability is limited to **packaged text** (a lesson, a summary, a draft of proposals) — never cfgs, never code, nothing that becomes wuxing. It generates *output*, never *infrastructure*. **No code sessions.** Its output ends at a **proposal**; a human disposes — the agent never holds a publish or side-effect capability.

### The scratch space — the primary boundary

Each agent gets a **scratch space**: a sandboxed working directory it reads and writes freely *inside*, with no path out except handing a finished artifact to a governed tool. Two layers: **free inside the sandbox, gated at the exit.** This is what makes the agent safe by construction — *safe because of the room, not because the worker is trusted.* Intermediate files (a trend digest, draft notes) live and die in the play area; nothing there is canonical.

### Capability surface & the two action lanes

- **Input** — multimodal (text + **vision**; image refs in the payload). **Output** — **structured** (schema in cfg) or free.
- **wuxing tools** — called over the bus, **gated by the allowlist**. Anything with side effects (write, send, spend) goes through here.
- **provider built-in tools** — the model's own tools (web search, etc.) that execute inside its turn. The **ungoverned read-only lane**: they don't cross the bus, so they're outside the allowlist, visible only in the transcript. Fine for *reading* the public web; never for side effects.

### Governance — capability-based security

The allowlist is a field in the **service cfg** (config is the program), and it's the agent's granted capability set, **enforced by wuxing at the bus** — mechanism, not persuasion. So a read-only job is *structurally* incapable of writing: if the loop emits "connector: write," wuxing refuses before it reaches the connector. **Two gates** per action: **authorization** (on the allowlist? — static) then **admission** (fits resources now? — scheduler).

- **Grain (resolved):** **coarse by default** (per capability: *may call web-search*), **fine where dangerous** (per operation + limits: *read-only on mtg-db, max 20 calls*). You pay the specification cost only where the blast radius justifies it — writes, deletes, spending, spawning.
- **Two edges:** the guarantee is only as good as the grain (read-can't-write needs a grant that distinguishes read from write), and it guards the *boundary*, not the inside — it assumes the sealed container's only exit is the gated bus.
- **Allowlist vs context:** both live in the cfg but serve different parties — context is what you *tell the model* (advisory, unenforced); the allowlist is what *wuxing permits* (enforced). The model owns the first; wuxing owns the second; only the second is a guarantee.

### Agent-authoring is excluded by construction

Not deferred — **incoherent under the design.** The allowlist lives in the cfg; if an agent could write a cfg, it would write its own permissions and grant itself everything. The author of a permission file can't be bound by it. And a cfg isn't user data — **anything indexed becomes canonical wuxing**, so writing one is *editing the system*, a privileged act. An agent writing a cfg would be the system rewriting its own permissions from inside an unprivileged loop. So agent-authoring is out, and **the escalation machinery is retired** — with authoring human-only, allowlists are simply human-written grants, full stop. (A future self-improvement, if it ever comes, must be a *different shape* — the agent *proposes*, a separate privileged step turns it into canonical config, the permission grant never in the agent's hands. It would arrive as a *new service*, not a new architecture.)

### Backend — Codex CLI

The agent/model backend is **Codex CLI** against the **ChatGPT-included window** (strong models, reserving Claude for personal coding). Codex is a coding-agent harness, but with no code sessions today its file/shell powers stay **leashed** — run sandboxed, with writes confined to the scratch space. Verified capabilities used: **headless** (`codex exec`, JSONL via `--json`), **per-step observable** (reasoning/commands/web-search/status in the stream), **`--output-schema`** (structured output), **image input** (`-i`, verify in exec), built-in **web search** (the read-only lane), **MCP** (to expose wuxing tools as *gated* actions if wanted), and **local models** (`--oss`).

## The scheduler

Resource-aware scheduling in the kernel — the only thing seeing total resources, every session, and the queue. The **job** (a script run), not the service, is the unit of scheduling and sizing.

**Two finite resources, opposite refill behaviour** (the dual-resource model):
- **Memory** — frees **on completion**. A finished job hands its GB back instantly.
- **AI quota** — the ChatGPT **5-hour rolling window**, shared across all AI sessions, refilling **on a clock**, *not* freed when a job finishes.

Mechanics:
- **Envelope per job (cfg):** **request** (floor, for admission) and **limit** (burst ceiling). Applied to the container at launch — one image runs capped per script.
- **Admission:** a job runs only when its **full request** fits — **never partial** (deadlock risk). An AI job must *also* have window left — two resources, both must pass. When nothing fits, jobs **queue** (the back-pressure valve); the box never over-commits.
- **Order:** priority class first (user > background), FCFS within class. **Backfill:** smaller waiting jobs fill gaps too small for the head, if it doesn't delay it.
- **Patience (cfg) + on-expiry:** each job declares **max_wait** and what happens on expiry — **escalate** (priority ages up, freed resource held/accrued for it), **overclock** (below), or **fail/drop**. Lean default: escalate. This is the same valve for *memory* waits and *quota* waits — when the window's exhausted, AI jobs queue on quota and their patience decides wait-for-refill vs fail.
- **Overclock — burst into reserve.** One memory pool, two admission lines: a **soft budget** for normal jobs, a **reserve band** only privileged work (user-triggered or starving) may enter, up to the **hard ceiling** — never past. Drawn **dynamically at actual request**. Normal jobs are fenced out because memory can't be reclaimed mid-job.
- **Window-state tracking:** the kernel tracks AI quota (messages used, time to refill) as scheduling state, the way it tracks free memory; Codex's `--json` usage makes it observable, and the tracking tables let you watch burn rate and predict exhaustion.

## Observability — king

**Trace everything, at every layer** — no blind spot. Captured **by construction**; wuxing is a **faithful recorder** (no built-in dashboards; analysis is human, in SQL). Properties: **completeness** (record what can't be reconstructed), **correlation** (shared keys — `session_id`, job id, workflow-run id), **queryability** (SQL fact store, not grep).

**Three emitters:** **services** (sessions + functions), **tools** (tool-calls), **wuxing itself** (scheduling decisions — admitted/queued, wait, overclock, budget, *and* AI-window state).

**Logs vs tables, both:** tables = structured facts for aggregates (you query); logs = per-run narrative for forensics (you read).

**Four table families:** **data** (domain payloads — the product), **tracking** (the trace — observed state), **log** (narrative — forensics), **admin** (index/manifest/configs — declared state; admin = what it *is*, tracking = what it *did*).

**Tracking schema** (trace/span, joined by `session_id`): **session** (parent — total peak/avg memory, duration, queue wait, priority, burst, exit, placing keys; container-level, free), **function** (child — per script/function peak/avg, duration, order, own id; needs in-service spans), **tool-call** (per invocation — tool, op, latency, status + typed detail: ai → model/tokens/cost/TTFT, connector → backend/rows/query-time). Grains **reconcile, don't double-count** (parent peak ≠ sum of children).

**Retention:** per-second usage series is **live-only** (sampled for peak, discarded); what persists is **one summary row per script per run**. Small, kept indefinitely. Aggregates make the system self-tuning (set honest requests/limits, right-size the reserve, predict window exhaustion).

## Clients & the operator surface (control plane)

Clients consume the kernel's **control interface** — reading state and issuing commands (like `kubectl`) — the **control plane**, deliberately separate from connectors (the *data plane*, services moving data to sinks). A GUI talks to the kernel's control interface, **not** through the I/O tool, for the same reason the cockpit isn't a connector. Clients: the **TUI** now (cockpit aesthetic), a **GUI** later, the `wuxing new` builder front-door, and eventually the **pixel world** (a character's state = its service's status) — all views onto the same kernel state.

**Two halves of operating.** Clients are for when *you go look*. For when the system must **summon you** — a proposal waiting, a run halted, the AI window nearly spent — those are internal **bus events**, so alerting is just the **messenger firing on an internal event, pointed at you** (the MTG-notifier mechanism aimed inward). The TUI is pull; the messenger is push. Both already exist in the architecture.

## Evolution & lifecycle

**Changing a service — drain-then-swap.** Editing a service is **deindex → change → reindex**; reindex triggers the **library** to verify integrity (cfg matches its files, the image rebuilds clean). The rule that sidesteps versioning for v1: a service **can't be reindexed while it has running instances** — you **drain first** (let in-flight runs finish, or stop them), then swap. There is only ever *one* live version, and it can't be changed out from under a running job. (Hard-save the whole service now; dynamic folder recognition, and versioning proper — keeping and pinning old versions — are later luxuries.)

**Supervising the daemon.** wuxing is launched by hand while building, then run as a **boot service unit** (systemd on the VM, or a restart policy if wuxing itself runs in a container). wuxing watches its services; the **init system watches wuxing** — the stack bottoms out at the OS, which is correct: the supervisor of everything is itself supervised by the OS.

## Flow control

**Triggers (initiation):** wuxing *watches* and fires — **schedule** (cron) or **event** (bus-event rules); conditions are cfg, watching is kernel substrate; a DB change arrives as a **bus event**, not a poll. **Orchestration (inside a service):** graph drives that service's internal DAG, awaiting confirmations. **Event coupling (between services):** a service emits an event on finish; another, watching for it, is triggered — the *only* inter-service mechanism, indirect and one-way (the rule, not an exception). **Triggers begin and bridge; graph progresses inside.**

## Worked example — MTG new-set notifier

*Setup once:* an **MTG service** (folder: checker + scraper + updater scripts + cfg) whose **internal workflow** is *check Scryfall → branch on `new_set` → scrape → write mtg-db*, emitting `mtg-db updated` on success. A separate **notifier service** is **triggered** by that event. `wuxing new` builds one image per service; index live. Creds in `.env`.

*A run:* cron fires → wuxing opens a **session**, runs the **MTG service** container → inside, it checks Scryfall (its own script), and on a new set scrapes and writes mtg-db via a *connector* (the write mints a **crossing-fact** + a `mtg-db updated` **event**) → its high-water mark advances only now, on success → wuxing's **trigger** sees the event and runs the separate **notifier service**, which composes via the **ai tool** and ships to Discord. The two services never touched — only the event bridged them. Every step emitted tracking rows.

*Second example — social media service (the agent model):* a **two-step workflow**, both AI. **Gather** — an agent uses the provider's web search to find trends and summarizes them to an md *in the scratch space*. **Propose** — a second agent reads that md plus its context and outputs **proposals** (post topics, angles, illustration types) as drafts. Two steps (not one) so each is seen, costed, and capped independently, and the gather model can be cheap while the propose model is strong. The md is churn — it never leaves the scratch space. The proposals **stop as drafts for your review**; publishing is a separate governed step you trigger. An autonomous manager that schedules and posts is a *future service* wuxing would launch when warranted — the same system at a higher capability level, added as content, not a rewrite.

## Build philosophy & architecture stance

**Borrow / build / command:** **borrow** the Hermes-shaped agent runtime and memory (model-agnostic gateway, session memory) — but *not* skill-writing-into-the-system, which is excluded by construction; **build** the uniquely-wuxing control plane (kernel/router, scheduler, observability, catalog + index gate, the governance model); **command** commodity infra (Docker, the model via Codex) — driven, never rebuilt.

**Non-monolithic in design, low-ops to start.** Run wuxing + tools as a **modular monolith** (one Go binary, in-process bus); split a piece out only when forced. The bus is **part of the kernel**.

**wuxing is kube-shaped, not kube:** a control plane driving a container engine over a fleet, but orchestrating **workflows** not containers (closer to **k8s + Argo**), an **interpreter** not a reconciler, **app-layer** not infra. Use Docker; treat k8s as a reference only.

## Open / deferred

- **Data modelling (the big next piece).** Detailed schema across the four table families — keys, columns, typed tool details. Includes: is a connector *call* (latency) the same record as a *crossing* (rows), or two?
- **Codex specifics to confirm** — image input in `exec` (headless) mode; whether to gate its tool-use via MCP or only observe its transcript.
- **"tool" naming overlap** — also an agent-callable function (MCP/ReAct); reserve **"action"** for that sense, or let it resolve.
- **Universal connector (north star)** — folding bespoke API access (today in service scripts: JS/Python/HTTP) into governed, declarative cfg, so even API reach becomes gated.
- **Full service versioning** — keeping/pinning old versions and dynamic folder recognition (v1 is drain-then-swap, one live version, hard-saved). Deferred.
- **Library name** · **catalog scope** (definitions vs templates) · **completeness display** (provisional vs final) · **intra-service workflow format** (how a service declares its internal DAG — the system's language) · **image build timing** (eager vs lazy) · **process-splitting** (which modules, when).

---

**Component set (current):** **wuxing** (kernel: router + bus/queue/sessions/running-registry/trigger-watching/**dual-resource scheduler** + state interface) · **tools** (library, graph, processors, connectors, ai — compiled, in the manifest) · **services** (sealed boxes — content, containers, in the index) incl. **first-party** suite · **container engine** (Docker — assumed dependency) · **agent backend** (Codex CLI — driver) · **clients** (TUI/GUI — control plane, messenger-as-alerting) · **OS** (supervises the daemon).
