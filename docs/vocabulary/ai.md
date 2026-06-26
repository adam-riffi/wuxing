# ai — function vocabulary

The AI verb: run model work for a service in two modes — one-shot **infer** and
bounded **agent** sessions — and return a result. Generic and domain-blind:
prompt, context, model, mode, schema, and (agent mode) the allowlist all come
from the caller's cfg. Backend: Codex CLI against the ChatGPT-included window.

Every call emits an `ai` detail fact — model, backend, mode, `tokens_in/out`,
**`cost` (recorded at incur-time, not derived)**, `ttft_ms`, `turn_count` — the
cost / window-burn lane.

## Common parameters

| Param | Type | Meaning |
|---|---|---|
| `model` | selector | which model/backend |
| `context` | text/refs | advisory context given to the model (unenforced) |
| `payload` | multimodal | text + vision (image refs) |
| `schema` | JSON schema (opt) | constrains structured output |

## Functions

| Function | Mode | Inputs | Output |
|---|---|---|---|
| `infer` | one-shot | `payload`, `context`, `model`, `schema?` | free text or schema-constrained |
| `autocomplete` | one-shot (infer-family) | `payload`, `model` | text completion |
| `agent` | session loop | `payload`, `context`, `model`, `allowlist`, `envelope` | a **proposal** artifact |

### `infer`

One-shot completion (payload → result), for generation and summarization.

- **Inputs:** multimodal `payload`, `context`, `model`, optional output `schema`.
- **Output:** free text, or structured output validated against `schema`.
- **Allowlist:** the caller must hold an `ai.infer` grant; refused at the bus otherwise.
- **Envelope + facts:** tool-call envelope; `ai_ft_call` detail (model, tokens, `cost` at incur-time, `ttft_ms`, `turn_count = 1`). Cost is recorded even when output fails schema validation.

### `autocomplete`

An infer-family primitive for short text completion. Same inputs/outputs/facts
as `infer` at lower ceremony (no schema).

### `agent`

A model-driven loop (think → act → observe → decide) for tasks whose next step
isn't declarable up front. The agent is a **bounded content producer**: it
writes only packaged text (lessons, summaries, **proposals**), never cfgs or
code, runs **no code sessions**, and ends at a proposal a human disposes.

- **Inputs:** `payload`, `context`, `model`, the **action allowlist** (which
  wuxing capabilities the agent may call over the bus), and a resource/patience
  **envelope** (request/limit + max_wait).
- **Output:** a proposal artifact in the scratch space (the only exit is handing
  a finished artifact to a governed tool).
- **Governance — two action lanes:**
  - **wuxing tools** over the bus — gated by the allowlist (coarse by default,
    fine for dangerous: per-op + limits). A side-effecting call not on the
    allowlist is refused before it reaches the tool.
  - **provider built-in tools** (web search, …) — the ungoverned read-only lane;
    transcript-visible, never for side effects.
- **Envelope + facts:** tool-call envelope per sub-call; `ai_ft_call` detail with
  `turn_count` > 1 and accumulated cost.

## Boundaries

No code sessions; no cfg/service authoring (excluded by construction — the author
of a permission file can't be bound by it); no unilateral side effects; no acting
past the proposal. It infers and proposes — it does not move data (connectors),
publish, or mutate the system.

## Open

The agent-mode entry's exact signature; whether agent tool-use is gated via MCP
or only observed via transcript; Codex `exec` + image confirmation.
