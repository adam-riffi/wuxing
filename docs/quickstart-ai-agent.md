# Quickstart — wuxing's AI (auto-detected agent CLIs)

wuxing's `ai` tool runs model work by driving an **agent CLI** that wuxing
**detects on your system** — like Hermes / Open Design auto-detecting agents. No
manual connection: install a supported CLI, and `wxg infer` just uses it.

Two modes:

- **chat** (`infer`) — one prompt in, an answer out.
- **agent** — hand a *brief* to a CLI that **drives itself** (plans, uses its own
  tools, writes files) in an isolated scratch dir; wuxing captures the artifact.

## 1. Install a supported agent CLI

wuxing auto-detects these on your PATH, in this preference order:

| Agent | Binary | Notes |
|---|---|---|
| Codex | `codex` | default invocation `codex exec "<prompt>"` |
| Claude Code | `claude` | `claude -p "<prompt>"` (print mode) |
| Gemini CLI | `gemini` | `gemini -p "<prompt>"` |
| Hermes Agent | `hermes` | prompt piped on stdin |
| Open Design | `od` | prompt piped on stdin |

Install any one (with its own auth — wuxing passes your environment straight
through, so whatever key the CLI already reads will work).

## 2. Check what wuxing found

```bash
go build -o wxg ./cmd/wxg
./wxg infer detect
```

```
Detected agent CLIs (default first):
* codex        /Users/you/.npm/bin/codex
  claude       /Users/you/.local/bin/claude
  hermes       /Users/you/.../hermes
```

The `*` is what `wxg infer` uses by default.

## 3. Talk to wuxing

```bash
./wxg infer chat "who was gorbachev"            # uses the detected default
./wxg infer chat "who was gorbachev" claude     # force a specific agent
./wxg infer agent "Draft a Discord post about today's new Magic set."
```

The answer prints to **stdout**; the agent used is noted on **stderr**
(`[wuxing: via codex]`). Flags: `--model <label>`, `--timeout <seconds>`.

The daemon has the same one-shot smoke command:

```bash
go build -o wuxing ./cmd/wuxing
./wuxing agent --brief "Write a haiku about a new Magic set."
```

## 4. Use it from a service

In a service cfg, call the `ai` tool's `agent` (or `infer`) operation:

```yaml
workflow:
  - id: draft
    tool: ai
    operation: agent
    with:
      brief: "Summarize today's new Magic set for a Discord post."
      # model: o4            # optional
      # timeout_seconds: 600 # optional
```

The step's result is the captured artifact (JSON if the agent emits JSON,
otherwise a JSON string). Each call records an `ai` fact (`mode = "agent"`), so
agent runs show up in the spine and cost lanes next to `infer` calls.

## Overriding detection (optional)

You only need this if a CLI's flags differ from wuxing's defaults, or you want a
custom command/wrapper. Set `WUXING_AI_AGENT_*` (see `.env.example`):

| Variable | Meaning | Default |
|---|---|---|
| `WUXING_AI_AGENT_COMMAND` | the binary to run (takes precedence over detection) | — |
| `WUXING_AI_AGENT_ARGS` | extra args, space-split; `{{brief}}` `{{model}}` `{{workdir}}` `{{output_file}}` expand | — |
| `WUXING_AI_AGENT_PROMPT_VIA` | `stdin` \| `arg` \| `file` | `stdin` |
| `WUXING_AI_AGENT_OUTPUT_VIA` | `stdout` \| `file` | `stdout` |
| `WUXING_AI_AGENT_OUTPUT_FILE` | filename in the scratch dir when `OUTPUT_VIA=file` | — |
| `WUXING_AI_AGENT_USAGE_FILE` | optional JSON usage sidecar | — |

When `WUXING_AI_AGENT_COMMAND` is set, `wxg infer chat "…"` (no backend named)
uses it instead of auto-detecting.

## Notes & limits

- **Detection finds the binary; the invocation is a sensible default.** If your
  CLI's headless flags differ from the table above, override with
  `WUXING_AI_AGENT_*` or wrap the CLI in a small script on your PATH.
- **Usage/cost.** Most CLIs don't print tokens/cost. To capture them, have your
  CLI (or wrapper) write a `WUXING_AI_AGENT_USAGE_FILE` JSON sidecar in the
  workdir: `{"tokens_in":1200,"tokens_out":340,"cost":0.012,"turns":5}`. Without
  it the fact still records the call (`turns=1`, zero cost).
- **Sandboxing.** The scratch dir is a fresh temp dir per call, removed after.
  wuxing bounds *what it runs and for how long*, not what the agent's own tools
  can reach — run agents you trust, or run the daemon in a container.
