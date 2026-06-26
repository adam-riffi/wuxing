# connectors — function vocabulary

The data-I/O verbs. Each moves data across the service boundary through a
**driver per backend** (selected by `type` in cfg) using `.env` credentials, and
is **gated by a data-access grant** (`DATABASE.TABLE`) enforced at the boundary.
Every call is metered: it emits a crossing fact, and a mutating write also mints
a data-mutation event.

Connectors move **references**, not inlined contents — a `payload_ref` points at
rows/a file the kernel already tracks.

## Common parameters

| Param | Type | Meaning |
|---|---|---|
| `type` | enum (`sqlite`, …) | selects the backend driver |
| `target` | `DATABASE.TABLE` | the scoped destination/source; also the **grant** authority |
| `creds` | secret ref (`${VAR}`) | `.env` reference, never the value |
| `payload_ref` | ref | rows or file the data lives in (not inlined) |
| `destination` | enum (`machine`, `human`) | machine → a sink/table; human → a kept artifact |

## Functions

| Function | Direction | Inputs | Output | Mutating |
|---|---|---|---|---|
| `read` | inbound | `target`, `filter?`, `payload_ref` (out) | rows ref + count | no |
| `query` | inbound | `target`, `sql` (ref) | rows ref + count | no |
| `write` | outbound | `target`, `payload_ref`, `mode` (`insert`/`append`) | ack + row counts | yes |
| `upsert` | outbound | `target`, `payload_ref`, `keys` | ack + row counts | yes |

### `read`

Read rows from `target` (optionally filtered) into `payload_ref`.

- **Inputs:** `target` (`DATABASE.TABLE`), optional `filter`, output `payload_ref`.
- **Output:** a rows reference + `rows` count.
- **Grant:** read grant on `target`; a grant to `mtg.cards` cannot read `mtg.users`.
- **Envelope + facts:** tool-call envelope (`grant_ref = target`); `connector_ft_crossing` (direction `in`, `rows`, `bytes`, `query_ms`). Not mutating — no data-mutation event.

### `query`

Run a read-only `sql` statement against `target`'s database, returning rows.

- **Inputs:** `target` (database scope), `sql` (statement ref).
- **Output:** rows reference + `rows` count.
- **Grant:** read grant on the database scope; rejected if the statement writes.
- **Envelope + facts:** as `read`. (Open: whether `query` and `read` unify — see the architecture doc.)

### `write`

Write `payload_ref` to `target`.

- **Inputs:** `target`, `payload_ref`, `mode` (`insert` | `append`).
- **Output:** ack + `{rows_inserted}`.
- **Grant:** write grant on `target` — least privilege, declared in cfg, enforced by the connector. Out-of-grant write is refused at the boundary before any row is touched.
- **Envelope + facts:** tool-call envelope (`grant_ref = target`); `connector_ft_crossing` (direction `out`) **and** `wuxing_ft_data_mutation` (`rows_inserted`/`updated`/`deleted`). Highest-volume facts in the system.

### `upsert`

Insert-or-update `payload_ref` into `target` keyed by `keys`.

- **Inputs:** `target`, `payload_ref`, `keys` (the conflict columns).
- **Output:** ack + `{rows_inserted, rows_updated}`.
- **Grant + facts:** as `write`.

## Boundaries

Connectors don't decide *what* moves (the service does), don't store domain data,
and don't author destinations beyond the granted `target`. Uniform backends only;
the long tail of bespoke APIs lives in service scripts today (the universal
connector is the north star). Both services (cfg steps) and agents (granted,
bus-routed actions) call connectors, metered identically.

## Open

The exact final verb set (does `query` fold into `read`? does `upsert` fold into
`write`?), per-driver detail tables, and the crossing-vs-mutation redundancy
(two views of one act) are confirmed during driver implementation.
