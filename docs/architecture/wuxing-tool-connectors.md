# wuxing — connectors tool

*Data I/O across the boundary, via a driver per backend. Also the data-plane meter.*

> **Status:** responsibility and boundaries **settled**; precise **function vocabulary is the pending derivation** (one of the heavy tools whose signatures the core's interpreter and the cfg grammar wait on). Mid-level shape below; exact `read`/`write`/etc. signatures TBD in the vocabulary pass.

## Responsibility (the one job)

Move data between a service and an external backend (SQL databases, sinks, APIs), through a **driver per backend** selected by `type` in cfg, using `.env` credentials. Connectors are also the **data-plane meter**: every crossing emits a movement-fact (audit + volumetry). *Don't meter the meter.*

## Surface — functions (provisional)

The shape, not the final names: an **outbound** verb (write/send to a target) and an **inbound** verb (read/query from a source), each parameterised by target, operation, and payload reference. Likely the highest-vocabulary tool alongside ai — the precise function set (read, write, query, upsert…?) is exactly what the vocabulary pass must pin, because each becomes cfg grammar.

## Inputs & outputs (incl. emitted metadata)

In: a target (`DATABASE.TABLE` or sink id), an operation, a payload reference (connectors move *references*, not inlined contents). Out: rows/results inbound; an acknowledged write outbound. **Emitted metadata** (the part that's load-bearing for the data model): a **crossing fact** per movement — backend/driver, target, direction (in/out), `rows`, `bytes`, `query_ms` — and, on a mutating write, it is the act that mints a **data-mutation** event (counts of rows inserted/updated/deleted). Highest-volume fact table in the system.

## Config surface

A service's cfg declares, per connector use: the backend `type` (selects the driver), the secret **reference** for creds (`${...}`, never the value), the **data-access grant** `DATABASE.TABLE` (the authority — least-privilege, naming exactly which db/project and table may be touched), the operation, and the **output destination** intent (machine-payload → a sink/table, vs human-artifact → a kept file location).

## Boundaries (what it does *not* do)

Does not decide *what* moves (the service does), does not store domain data (it's transport + meter), does not interpret cfg (core), does not author destinations beyond the granted `DATABASE.TABLE` — a grant to `mtg.cards` cannot write `mtg.users`. Today it handles **uniform** backends; the **long tail of bespoke APIs lives in service scripts** as an explicit interim, with a **universal connector** (API reach folded into governed cfg) as the north star.

## Callers

Services (as workflow steps in cfg), and **agents** (as a granted, bus-routed action) — both gated by the data-access allowlist, both metered identically. The metadata write itself is automatic and universal (the I/O path); only the *destination* is the declared, scoped part.

## Open / deferred

The full function vocabulary and per-driver detail tables; the **crossing-vs-mutation** redundancy (kept as two views — movement vs state-delta — pending a conscious confirm); the universal-connector design (deferred end-product feature).
