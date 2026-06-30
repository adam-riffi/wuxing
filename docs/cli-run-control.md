# Run control — targeting, inputs, and forced sequences (design)

How an operator drives runs with precision: launch only part of a service, inject
the inputs a partial run needs, and force a specific chain of runs. This is the
design for the `wxg run` command and the daemon control channel it needs. The
observability side (`wxg runs` / `wxg show`) is built; this is the *control* side.

Two motivating cases:

1. *"Launch a service but only run its second job."*
2. *"Force a specific sequence of runs."*

Both decompose into a small set of orthogonal parameters, and — importantly —
**most of the kernel primitives already exist**; this is mostly about *exposing*
them.

## The parameter axes

A run invocation varies along four independent axes:

### 1. Targeting — *which steps run*

A service's workflow is ordered steps, each with an `id` (already in the cfg
model). Default is the whole workflow; you narrow it:

| Flag | Meaning |
|---|---|
| *(none)* | run the full workflow |
| `--step <id>` | run exactly one step (case 1: "only the second job") |
| `--from <id>` | start at a step, run to the end |
| `--to <id>` | run from the start up to a step |
| `--only <id,id,…>` | run a specific subset |

*Backed by:* steps already carry `id`s; the interpreter's `Run` gains a start /
stop / filter bound. A single call is already a one-step workflow.

### 2. Inputs — *what a partial run starts from*

If you skip step 1 but run step 2, step 2's upstream inputs are missing. So a
partial run must be **seeded** with the facts it would otherwise have received:

| Flag | Meaning |
|---|---|
| `--with key=value` | inject one input fact (repeatable) |
| `--input @file.json` | inject a fact object from a file |

*Backed by:* the cfg already has per-step `with:` args, and the interpreter
already merges *(accumulated facts, step.With)* via `buildPayload`. Injected
inputs simply seed the "accumulated facts" the (partial) run begins with — no new
mechanism, just an entry point.

### 3. Sequencing — *causality and the cascade* (case 2)

By default a manual run opens a **new** sequence and, on completion, fires its
successors (the auto-cascade). You override that to script a chain:

| Flag | Meaning |
|---|---|
| `--sequence <id>` | attach this run to an **existing** sequence (inherit it) instead of opening a new one — forces it into a causal chain |
| `--no-cascade` | run the service but **do not** fire its successors (stop the auto-cascade) |
| `--then <service>[,…]` | force specific successor runs regardless of their conditions — a hand-scripted cascade |

*Backed by:* event triggers already **inherit** a sequence (vs external opening a
new one); the Runner already fires successors via `OnEvent`. Forcing = bypass the
successor condition eval and fire the chosen ones; `--no-cascade` = skip firing.

### 4. Execution mode — *how it runs*

| Flag | Meaning |
|---|---|
| `--dry-run` | show what *would* run (steps + which successors fire) without executing — like `kubectl apply --dry-run` / `terraform plan` |
| `--wait` / `--detach` | block for the result vs fire-and-return |
| `--priority N` / `--force` | scheduler hints (the scheduler already has priority + an overclock reserve band) |

## Composing the two cases

**Case 1 — only the second job, fed its inputs:**

```bash
wxg run mtg --step compose --with new_set=true
```

Runs just the `compose` step, seeded with the fact the upstream `check` step would
have emitted. One run, one step, no cascade unless asked.

**Case 2 — a forced sequence of runs.** Two ergonomic options:

*a) Manual chain* — share one sequence across explicit calls:

```bash
SEQ=$(wxg run mtg --no-cascade --print-sequence)
wxg run notifier --sequence $SEQ --no-cascade
wxg run archiver --sequence $SEQ
```

All three land under one `sequence_id` (one causal chain in the spine), in the
order you fired them, regardless of successor conditions.

*b) Plan file* — declare the forced order once and let the daemon run it:

```yaml
# plan.yml — a scripted, forced sequence
sequence:
  - service: mtg
    step: check
  - service: notifier
    with: { new_set: true }
  - service: archiver
```

```bash
wxg run --plan plan.yml
```

The plan is the clean primitive for *scripted orchestration* — closer to a CI
`workflow_dispatch` / Airflow manual DAG run than to anything in docker/kube.

## What this needs that isn't built yet

- **The daemon control channel (RPC).** Targeting/inputs/sequencing all *mutate
  live state*, so `wxg run` must reach the running kernel (a local socket / named
  pipe). This is the one real prerequisite — the same channel the catalog
  (`library index/deindex`) and a TUI's "drive" features need. See the keystone
  note in the command audit.
- **Interpreter step bounds.** `Run` needs `--step` / `--from` / `--to` filtering.
- **A run-request type** carrying the four axes, threaded from `wxg run` → RPC →
  `Kernel.Run` / the Runner.

Everything else (step ids, `with:` merge, sequence inheritance, successor firing,
scheduler priority) is already in the kernel — so this is surfacing, not
inventing.

## Open questions

- **Partial-run lineage:** does a `--step compose` run record *which* steps ran
  (a partial-run marker on the run fact), so the spine stays honest? (Recommended:
  yes — stamp the targeted range.)
- **Forcing vs safety:** should `--then` / `--force` require a confirmation or a
  flag, since they bypass the cfg's declared conditions?
- **Plan grain:** is a plan a first-class stored artifact (re-runnable, in the
  catalog) or a throwaway file?
