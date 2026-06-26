# wuxing — data model (service-agnostic core)

*Companion to the system doc. Covers only the data wuxing itself owns — the platform's own record, uniform across every fleet. Service/domain data (the cargo) is out of scope.*

> **Status: foundational, not final.** The architecture below is settled — frame, tiers, rules, the lineage spine, naming. The **per-tool detail tables are deliberately deferred**: metadata records what a tool *does*, and the tools' functionality isn't designed yet. The final model depends on each tool's eventual operations and internals. *Metadata follows functionality, not the other way around.* Treat the per-tool fields here as illustrative placeholders, the spine and rules as committed.

## The governing metaphor — the bureaucrat

wuxing is a **records office**, not a warehouse. It records *that* a thing happened — when, by whom, under what authority, to what effect, at what cost — and never the contents. It keeps the ledger and the paperwork; the **cargo stays with whoever shipped it.** The test for any field: *would a bureaucrat record this?* Timing, identity, authority, outcome, volume — yes. What the data *was* — no. If a column describes contents rather than that-an-event-occurred, it's crossed into service-specific territory and doesn't belong.

**The bright line:** the service generates work and content; **wuxing generates the record of it.** A service reports a domain fact (a value) and produces content (cargo); everything else — ids, ordering, timing, resource profile, outcome, authority — is **wuxing's, stamped from the outside.** The observed never authors the observation. That's what makes the metadata trustworthy: it's recorded by the observer, not self-reported. A service never writes to a metadata field, so it can't forge its lineage or lie about its position.

## Founding rules

- **Metadata is append-only; data is mutable.** Metadata is the record of what *happened* — history, fixed the moment it occurs; you never rewrite an event, you file a correcting one. Data is *state* — a present value the service owns and may update/delete.
- **Every mutation of data is itself an append-only metadata event.** Data moves freely, but each change leaves a permanent footprint in the record. History accumulates; state evolves; the link is never broken.
- **State vs event — record transitions and incidents, not steady state.** "Is it running?" is a **live read**, never stored. Crashes, starts, completions, admissions are **events** — stamped. Sitting idle is the *absence* of events — recorded as nothing; the gap in the log *is* the information. No diary of "still running, still idle."
- **wuxing assigns all metadata.** Order, lineage, timing — all stamped by the kernel, the only thing that sees the whole picture. (Subsumes "the parent assigns the child's order.")

## The four storage tiers

Each answers a different question:

1. **Mutable state** — current values (a vault, the latest set list). Services own it; may change. *What is now.*
2. **Append-only tables** — the structured event/metric ledger (facts + change-tracking). Small, queryable, kept indefinitely (summary grain). *What's the pattern.*
3. **The artifact store** — services' actual flat-file outputs (md, json, drafts). **Referenced by the tables, never ingested** — wuxing records path + metadata, the file lives on a filesystem. *The produced deliverables.*
4. **Scratch space** — per-agent ephemeral churn, swept each run, never kept. *Throwaway workspace.*

The scratch/artifact line is **destination, declared in cfg**: a service emits an output as a *machine payload* (bound for a `DATABASE.TABLE` sink) or a *human artifact* (a kept document) — wuxing tracks both uniformly; only the service knows what the output is *for*. Scratch is swept by default; an artifact exists only when a service deliberately emits it.

## Ordering & lineage — causal order as data

Order is **explicit, never inferred from clocks** (parallel steps share a timestamp; clocks have resolution limits; "what triggered what" is causal, not temporal). Two complementary stamps on every event:

- **Lineage ids** — *what belongs to what.* The case-file hierarchy, five deep, outermost to innermost:

  `sequence_id → run_id → session_id → call_id → tool_function_id`

  - **sequence** — a causal chain of runs linked by event triggers (one origin trigger cascading across services)
  - **run** — one triggered execution (one service, end to end)
  - **session** — a unit of work within a run
  - **call** — a tool invocation within a session
  - **tool_function** — an internal step within a call

- **Order fields** — *position within the parent*, an integer rank: `sequence_order, run_order, call_order, step_order`.

Together: filter by `sequence_id`, UNION the event rows across every fact table, `ORDER BY sequence_order, run_order, call_order, step_order DESC` — and read the entire cascade in causal order, top to bottom, across services/calls/steps, **with no timestamp involved.** `started_at`/`ended_at` are kept for *duration*, never for *what-came-first*.

**Sequence propagation:** a sequence id must travel *across the event boundary* — the bus event triggering the next service **carries the `sequence_id`**, so the next run inherits it rather than starting fresh. Rule: an **external** trigger (cron/manual) opens a **new** sequence (chain head); an **event** trigger **inherits** the sequence (a link). This is the one new mechanism the model requires of the bus.

## Naming & ownership

`‹database›.‹domain›_‹kind›_‹entity›`

- **database** — always `wuxing`; everything the platform owns lives here, separate from service data.
- **domain** — who owns the table: `wuxing` (the core/kernel — the spine) or a tool (`graph`, `agent`, `connector`, `library`, `processors`). The doubled `wuxing.wuxing_` is accepted: the core is just another domain named `wuxing`.
- **kind** — `ft` (fact: an event; append-only; grows; counted/measured) or `dt` (dimension: a small, stable lookup joined to facts).
- **entity** — what it records.

So `wuxing.agent_ft_session` = "wuxing database, agent domain, fact table of sessions." **The kernel owns the case-file spine; each tool owns the record of what it did** — and a tool's whole footprint is everything under its `‹domain›_` prefix. The rule of thumb: *add a row every time something happens → `_ft_`; a list you look things up in → `_dt_`.* Every future table places itself automatically.

## The spine (the `wuxing` core domain — settled)

Each fact carries the lineage stamp `‹lineage›` (the ids + order fields that apply). Nothing holds contents — only refs, measures, authority, outcome.

`wuxing.wuxing_ft_sequence` — one row per causal chain
- `sequence_id` (PK) · `origin_trigger_id` (FK) · `opened_at` · `closed_at` · `outcome_id` (FK)

`wuxing.wuxing_ft_run` — one row per triggered execution (a **fact**, parent-child to sequence; unbounded runs per sequence, no hardcoded max)
- `run_id` (PK) · `sequence_id` (FK) · `sequence_order` · `service_id` (FK) · `trigger_id` (FK) · `opened_at` · `closed_at` · `outcome_id`

`wuxing.wuxing_ft_session` — one row per unit of work
- `session_id` (PK) · `run_id` (FK) · `sequence_id` · `run_order` · `opened_at` · `closed_at` · `outcome_id`

`wuxing.wuxing_ft_scheduler_event` — one row per admission decision
- `event_id` (PK) · `‹lineage›` · `decision_id` (FK — admitted/queued/escalated/overclocked/rejected) · `at` · `mem_free_at_decision` · `ai_window_remaining` · `wait_ms`

`wuxing.wuxing_ft_session_resource` — the container's load profile per unit of work (wuxing observes from outside)
- `session_id` (PK/FK) · `‹lineage›` · `mem_peak` · `mem_avg` · `cpu_avg` · `did_overclock` · `request_mem` · `limit_mem`

`wuxing.wuxing_ft_artifact` — a service produced a file (tracked, never ingested)
- `artifact_id` (PK) · `‹lineage›` · `path_ref` · `type` · `bytes` · `destination` (human / machine) · `outcome_id`

**Core dimensions:** `wuxing.wuxing_dt_service` (id, name, version) · `wuxing_dt_trigger` (id, kind [cron/event/manual], spec) · `wuxing_dt_outcome` (id, name).

## The tool-call layer (envelope settled; details deferred)

The **envelope** is universal and committed — every tool call, regardless of tool, is stamped here, and it's where governance is audited:

`wuxing.‹domain›_ft_tool_call` (or one shared `wuxing.wuxing_ft_tool_call`) — one row per invocation
- `call_id` (PK) · `‹lineage›` · `tool_id` (FK) · `operation_id` (FK) · `parent_call_id` (FK→self, agent→tool nesting) · `grant_ref` (the **authority** — the allowlist entry or `DATABASE.TABLE` that permitted it) · `queued_at` · `started_at` · `ended_at` · `queue_wait_ms` · `exec_ms` · `latency_ms` · `outcome_id` · `error_ref` (→ archive, nullable)

The envelope answers "what calls happened, to what, when, did they pass, under what authority" uniformly — and it's where gated calls are audited, closing governance and observability on the same fact.

**Per-tool detail and internal-step tables are DEFERRED** to each tool's functional design. The *pattern* is fixed (envelope → typed detail → internal steps, all keyed by `call_id`); the *fields* aren't, because they describe operations not yet designed. Illustrative placeholders:

- `wuxing.ai_ft_call` — model, backend, mode, `tokens_in/out`, `cost` (stored at incur-time, not derived), `ttft_ms`, `turn_count`
- `wuxing.connector_ft_crossing` — backend/driver, `target_ref` (`DATABASE.TABLE`), direction, `rows`, `bytes`, `query_ms`
- `wuxing.‹domain›_ft_tool_function` — the tool's *own* internal steps (wuxing-instrumented, service-agnostic): `step_id` (FK→`‹domain›_dt_step`), `step_order`, timing, outcome — so a slow call decomposes into its phases (e.g. connect vs execute vs fetch)

The thin tools (`library`, `graph`, `processors`) get an envelope row and little or no detail at first.

## Change-tracking & data access

`wuxing.wuxing_ft_data_mutation` — change-tracking on mutable stores (summary grain), one row per mutating write
- `mutation_id` (PK) · `call_id` (FK — the write that caused it) · `‹lineage›` · `target_ref` (`DATABASE.TABLE`) · `rows_inserted` · `rows_updated` · `rows_deleted` · `at`

Applies **only to mutable stores** — never to append-only facts (which only grow by one row per event). Summary grain by default (counts); full row-level before/after CDC is a per-table luxury, reserved for where it earns its weight.

**Data access is an allowlist.** A service writes to SQL only via an explicit cfg grant — `DATABASE.TABLE` — naming the database/project and table. Same shape and enforcement as the agent tool-allowlist: least privilege, declared in cfg, enforced by the connector at the boundary, and recorded as the `grant_ref` authority on every write. A grant to `mtg.cards` can't write to `mtg.users`. Metadata tracking via the I/O tool is **automatic and universal**; the data *destination* is the declared, scoped part.

## Admin tables (declared state — noted, not facts)

The service index, the manifest, allowlists, trigger registrations — *what wuxing **is*** (declared, mutable), distinct from the event facts above — *what it **did***. Modelled later with the rest.

## Open decisions (within this layer)

- **Crossing vs mutation** — a connector write mints both a `crossing` (movement: rows out) and a `data_mutation` (effect: n inserted/updated/deleted). Two views of one act (volumetry vs state-delta); kept separate, but the one conscious redundancy to confirm.
- **Resource grain** — profile sits at `session` level (the container wuxing observes), not per-call, since container memory can't be cleanly attributed to one internal call. Finer is heavier.
- **Core domain echo** — `wuxing.wuxing_` accepted (not renamed `core`/`kernel`).

## What's deferred to tool design

Everything per-tool: each tool's detail fact(s), its internal-step vocabulary (`‹domain›_dt_step`), its operations (`dim_operation` entries), its measures. These are drawn **as part of building each tool**, when its operations and internals are real. The spine, the rules, the envelope, and the naming carry forward unchanged; the leaves grow with the tools.
