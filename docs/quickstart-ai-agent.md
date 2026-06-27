# Quickstart — drive an agent CLI from wuxing

wuxing's `ai` tool has two modes:

- **`infer`** — one-shot completion (prompt in, result out).
- **`agent`** — hand a *brief* to a real agent CLI that **drives itself** (plans,
  uses its own tools, writes files), and capture what it produces.

Agent mode is how you wire in [Hermes Agent](https://github.com/nousresearch/hermes-agent),
[Open Design](https://github.com/nexu-io/open-design), Codex, Claude Code, or any
other agent CLI. wuxing doesn't reimplement the agent — it **runs the CLI
headlessly in a sandbox and collects the artifact**, recording an `ai` fact
(model, tokens, cost, turns) like any other call.

## How it works

For each agent call wuxing:

1. creates an **isolated scratch directory** (the agent's workdir);
2. **delivers the brief** — via `stdin` (default), a positional `arg`, or a
   `file` in the scratch dir;
3. runs your `WUXING_AI_AGENT_COMMAND` there, **inheriting your shell
   environment** (so the CLI's own API key flows through), bounded by a
   **wall-clock timeout**;
4. **captures the artifact** — from `stdout` (default) or a named output file;
5. optionally reads a **usage sidecar** (`{tokens_in,tokens_out,cost,turns}`) and
   records the `ai` fact.

That's the whole contract. The CLI is allow-listed (one binary), sandboxed (one
scratch dir), and bounded (one timeout) — it "uses itself" inside those rails.

## 1. Install an agent CLI

Pick one and confirm it runs on your PATH. For example:

```bash
# Hermes Agent (Nous Research) — 300+ models via OpenRouter
#   see https://github.com/nousresearch/hermes-agent
hermes setup           # configure provider + key
hermes --version

# or Codex
codex --version

# or Open Design (spawns coding-agent CLIs to emit artifacts)
#   see https://github.com/nexu-io/open-design
od --version
```

> Each CLI has its own auth. Set its key in your shell (or `.env`) — wuxing passes
> your environment straight through to the child process, so whatever key the CLI
> already reads will work. For Hermes via OpenRouter that's `OPENROUTER_API_KEY`.

## 2. Configure wuxing

Set these in your shell or `.env` (see `.env.example`). Only `COMMAND` is
required:

| Variable | Meaning | Default |
|---|---|---|
| `WUXING_AI_AGENT_COMMAND` | the binary to run | — (required) |
| `WUXING_AI_AGENT_ARGS` | extra args, space-split; `{{brief}}` `{{model}}` `{{workdir}}` `{{prompt_file}}` `{{output_file}}` expand | — |
| `WUXING_AI_AGENT_PROMPT_VIA` | `stdin` \| `arg` \| `file` | `stdin` |
| `WUXING_AI_AGENT_OUTPUT_VIA` | `stdout` \| `file` | `stdout` |
| `WUXING_AI_AGENT_OUTPUT_FILE` | filename in the scratch dir when `OUTPUT_VIA=file` | — |
| `WUXING_AI_AGENT_USAGE_FILE` | optional JSON usage sidecar | — |
| `WUXING_AI_AGENT_MODEL` | model label recorded on the fact | — |
| `WUXING_AI_AGENT_TIMEOUT_SECONDS` | per-session wall-clock bound | `300` |

### Starting points per CLI

These are **templates** — confirm the exact headless flags for *your* installed
version, then adjust.

**Hermes Agent** (prompt on stdin, answer on stdout):

```bash
export WUXING_AI_AGENT_COMMAND=hermes
export WUXING_AI_AGENT_PROMPT_VIA=stdin
export WUXING_AI_AGENT_OUTPUT_VIA=stdout
export OPENROUTER_API_KEY=sk-or-...
```

**Codex** (brief as an argument):

```bash
export WUXING_AI_AGENT_COMMAND=codex
export WUXING_AI_AGENT_ARGS="exec"     # non-interactive subcommand
export WUXING_AI_AGENT_PROMPT_VIA=arg  # brief appended after the args
export OPENAI_API_KEY=sk-...
```

**Open Design** (agent writes an artifact file into the workdir):

```bash
export WUXING_AI_AGENT_COMMAND=od
export WUXING_AI_AGENT_ARGS="generate --out {{output_file}}"
export WUXING_AI_AGENT_OUTPUT_VIA=file
export WUXING_AI_AGENT_OUTPUT_FILE=artifact.html
```

> If your CLI is interactive-only (a TUI) or needs an unusual invocation, wrap it
> in a tiny shell script that reads the brief and prints the result, and point
> `WUXING_AI_AGENT_COMMAND` at the script. The placeholders make most CLIs work
> without a wrapper.

## 3. Smoke-test it

```bash
go build -o wuxing ./cmd/wuxing      # or: go run ./cmd/wuxing agent ...
./wuxing agent --brief "Write a haiku about a new Magic set."
```

The artifact prints to **stdout**; a one-line usage summary prints to **stderr**:

```
A set descends bright— / cards shuffle into the night / spoilers light the dawn
[wuxing agent: model=hermes turns=1 tokens=0/0 cost=0.0000 0ms]
```

`--model` and `--timeout <seconds>` override the spec defaults.

If you see `WUXING_AI_AGENT_COMMAND is not set`, your env isn't exported into the
shell running `wuxing`.

## 4. Use it from a service

In a service cfg, call the `ai` tool's `agent` operation and pass a `brief`:

```yaml
workflow:
  - id: draft
    tool: ai
    operation: agent
    with:
      brief: "Summarize today's new Magic set for a Discord post."
      # model: hermes-4          # optional
      # timeout_seconds: 600     # optional
```

The step's result is the captured artifact (JSON if the agent emits JSON,
otherwise a JSON string). Each call records an `ai` fact with `mode = "agent"`,
so agent runs show up in the spine and cost lanes next to `infer` calls.

## Notes & limits

- **Usage/cost.** Most CLIs don't print tokens/cost to stdout. To capture them,
  have your CLI (or wrapper) write a `WUXING_AI_AGENT_USAGE_FILE` JSON sidecar in
  the workdir: `{"tokens_in":1200,"tokens_out":340,"cost":0.012,"turns":5}`.
  Without it the fact still records the call (`turns=1`, zero cost).
- **Args with spaces.** `WUXING_AI_AGENT_ARGS` is split on spaces. For arguments
  that contain spaces, use a wrapper script.
- **Sandboxing.** The scratch dir is a fresh temp dir per call, removed after.
  The agent can still touch the rest of the system if it wants to — wuxing bounds
  *what it runs and for how long*, not what the agent's own tools can reach. Run
  agents you trust, or run the whole daemon in a container.
