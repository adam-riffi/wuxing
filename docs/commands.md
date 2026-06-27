# wuxing — command reference

What you can run today, and what each thing does. wuxing ships **two binaries**:

- **`wuxing`** — the kernel **daemon** (the long-running control plane).
- **`wxg`** — the operator **CLI** (the `kubectl` of wuxing; how you talk to it).

Build both (let `go build` name the output so you get the right extension per OS —
`wxg` on Linux/macOS, `wxg.exe` on Windows):

```bash
go build ./cmd/wuxing    # -> wuxing  or  wuxing.exe
go build ./cmd/wxg       # -> wxg     or  wxg.exe
```

Then run from the build dir: `./wxg …` on Linux/macOS, `.\wxg.exe …` on Windows
(or skip the build entirely with `go run ./cmd/wxg …`).

> **Windows note:** don't build with `-o wxg` — that writes an *extensionless*
> file, and Windows won't recognize it as a program (you'll get the "How do you
> want to open this file?" app picker). Use the commands above so the binary is
> named `wxg.exe`.

> Status legend: ✅ works · 🟡 partial · 🚧 stub (prints "not implemented")

---

## `wuxing` — the daemon

| Command | Status | What it does |
|---|---|---|
| `wuxing` | ✅ | Boots the kernel: loads the boot manifest, opens the fact store (SQLite, WAL), assembles the seven faces (bus, scheduler, sessions, triggers, interpreter, library, lineage) into a live control plane, then idles until `Ctrl-C` / SIGTERM and shuts down cleanly. |
| `wuxing agent --brief "…"` | ✅ | Runs one brief through an agent CLI that wuxing **auto-detects** on your PATH, prints the artifact to stdout and a usage line to stderr. The smoke test for an AI integration — no kernel needed. |

**`wuxing` flags**

| Flag | Default | Meaning |
|---|---|---|
| `--manifest` | `manifest/boot.yml` | Path to the boot manifest (the compiled tool list). |
| `--store` | `~/.wuxing/wuxing.db` | Path to the SQLite fact store (parent dirs auto-created). |

**`wuxing agent` flags**

| Flag | Default | Meaning |
|---|---|---|
| `--brief` | — (required) | The task to hand the agent. |
| `--backend` | auto-detect | Force a specific agent by name (`codex`, `claude`, …). |
| `--model` | — | Model label passed through / recorded on the fact. |
| `--timeout` | 300 | Wall-clock bound in seconds. |

---

## `wxg` — the operator CLI

### Global

| Command | Status | What it does |
|---|---|---|
| `wxg` / `wxg --help` | ✅ | Show the command tree. |
| `wxg --version` | ✅ | Print the build version. |

### `wxg infer` — talk to wuxing's AI

wuxing detects agent CLIs on your system and uses one with **no manual wiring**.
With no `[backend]` it auto-detects; name one to force it.

| Command | Status | What it does |
|---|---|---|
| `wxg infer detect` | ✅ | List the agent CLIs found on your PATH (the default is marked `*`). |
| `wxg infer chat "<prompt>" [backend]` | ✅ | One-shot completion. `wxg infer chat "who was gorbachev"` → runs the detected agent, prints the answer. |
| `wxg infer agent "<brief>" [backend]` | ✅ | Bounded agent session in an isolated scratch dir (the agent plans / uses tools / writes), prints the artifact. |

`[backend]` ∈ the detected agents (`codex`, `claude`, `gemini`, `hermes`, `open-design`). Flags: `--model`, `--timeout <seconds>`. The chosen agent is reported on stderr (`[wuxing: via codex]`); the answer goes to stdout.

### `wxg library` — the service catalog (🚧 stubs)

| Command | Status | What it will do |
|---|---|---|
| `wxg library index <service>` | 🚧 | Register a service (draft → live). |
| `wxg library deindex <service>` | 🚧 | Deregister a service (drains first). |
| `wxg library status <service>` | 🚧 | Drift: live folder vs the canonical snapshot. |
| `wxg library calls <service>` | 🚧 | Introspect the service's declared tool calls. |
| `wxg library functions <service>` | 🚧 | Introspect the service's declared script functions. |

These print `not implemented yet` today; they land when the CLI is wired to the live `library` tool over a daemon connection.

---

## Configuration (environment)

Copy `.env.example` → `.env`. Nothing here is required for AI — detection handles it.

| Variable | Purpose |
|---|---|
| `WUXING_AI_AGENT_COMMAND` (+ `_ARGS`, `_PROMPT_VIA`, `_OUTPUT_VIA`, `_MODEL`, …) | **Optional** manual override of the agent invocation, when a CLI's flags differ from wuxing's defaults or you want a custom command. See `docs/quickstart-ai-agent.md`. |
| `WUXING_FACT_STORE_PATH` | SQLite fact-store path (the daemon's `--store` default). |
| `WUXING_FACT_STORE_DSN` | Set to a Postgres URL to use a server backend instead of SQLite. |
| `DOMAIN_POSTGRES_DSN` | A connector's domain-data target (service cargo). |

---

## Not built yet

- **TUI** — there is no interactive terminal UI; `wxg` is the CLI surface. A build plan is in [`docs/tui-build.md`](tui-build.md).
- **Live `wxg library` / run commands** — the catalog/run subcommands are stubs pending a daemon connection.
- **Container service execution, Postgres execution** — need Docker / a Postgres DSN (see `CLAUDE.md`).
