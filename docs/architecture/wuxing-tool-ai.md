# wuxing — ai tool

*Infer and agency. The AI verb — bounded, governed, observable like any other tool.*

> **Status:** responsibility, modes, governance, and backend **settled** (system doc + this thread); precise **function vocabulary is pending derivation** (you named `infer` and `autocomplete` — the full signature set is the heavy-tool work the interpreter/grammar wait on).

## Responsibility (the one job)

Run model work for a service in two modes — **one-shot inference** and **bounded agent sessions** — and return a result. Generic and domain-blind: prompt, model, mode, schema, and the agent's allowlist all come from the caller's cfg. The ai tool **produces**; moving the result (vault, user) is a connector or the messenger.

## The two modes

- **infer** — one-shot completion (payload → result). For generation and summarization. Other named primitives (e.g. `autocomplete`) are infer-family.
- **agent session** — a model-driven loop (think → act → observe → decide), for tasks whose next step isn't declarable up front. Same scheduler/sessions/containers/bus as everything else; only the control logic differs.

## Surface — functions (provisional)

`infer` and `autocomplete` named so far; the agent-mode entry and any structured-output / multimodal variants are TBD. Each becomes cfg vocabulary, so the exact set is pinned in the derivation pass.

## Inputs & outputs (incl. emitted metadata)

In: **multimodal** payload (text + vision/image refs), context, model selector, output schema, and (agent mode) the action allowlist + resource/patience envelope. Out: free text or **schema-constrained** structured output. **Emitted metadata:** an `ai` detail fact per call — model, backend, mode, `tokens_in/out`, **`cost` (stored at incur-time, not derived)**, `ttft_ms`, `turn_count`. The cost/window-burn lane.

## Governance — capability security

The agent is a **bounded content producer, not an actor**: it produces packaged text (lessons, summaries, **proposals**), never code or cfgs, runs **no code sessions**, and ends at a proposal a human disposes — it never holds a publish/side-effect capability. Its **scratch space** is the primary boundary (free read/write inside, the only exit is handing an artifact to a governed tool). Two action lanes: **wuxing tools over the bus** (gated by an explicit per-service allowlist, coarse-by-default / fine-for-dangerous), and **provider built-in tools** (web search etc. — the ungoverned read-only lane, transcript-visible). Side effects always go through wuxing tools.

## Config surface

A service's cfg declares: prompt/context, model, mode (infer/agent), output schema, the **action allowlist** (which wuxing capabilities the agent may call), the resource/patience envelope, and the backend selector.

## Backend

**Codex CLI** against the **ChatGPT-included window** (strong models; Claude reserved for personal coding). Driven headless (`codex exec --json`, per-step observable), `--output-schema` for structured output, image input, MCP (to expose wuxing tools as gated actions), local-model fallback (`--oss`). Coding-agent powers stay **leashed** (no code sessions) — run sandboxed, writes confined to scratch.

## Boundaries (what it does *not* do)

No code sessions; no cfg/service authoring (excluded by construction); no unilateral side effects; no acting past the proposal. It infers and proposes — it does not move data (connectors), publish, or mutate the system.

## Callers

Services (as workflow steps), and the agent loop itself (sub-calls, each a governed request). An autonomous "manager" that publishes is a **future service**, not a future architecture.

## Open / deferred

Full function vocabulary; agent-mode entry shape; whether agent tool-use is gated via MCP or only observed via transcript; Codex `exec`+image confirmation.
