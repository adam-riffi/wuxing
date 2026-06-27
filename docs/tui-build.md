# Building a wuxing TUI — build instructions

There is no TUI today (`wxg` is a one-shot CLI). This is the plan to build one: an
interactive terminal cockpit for wuxing — chat with the AI, watch runs and lineage
stream by, and browse the service catalog. Follow the milestones in order; each is
shippable on its own.

## Decisions (settle these first)

| Decision | Recommendation | Why |
|---|---|---|
| Framework | **Bubble Tea** (`charmbracelet/bubbletea`) + **Lip Gloss** (styling) + **Bubbles** (textinput, viewport, table, spinner) | The Go-standard Elm-architecture TUI; batteries-included widgets. |
| Where it lives | `wxg tui` subcommand (not a new binary) | One operator surface; reuses `wxg` wiring. |
| How it reaches wuxing | **Read the fact store directly** for views; **call the `ai` package in-process** for chat | The daemon has no RPC server yet — don't block the TUI on building one. The fact store (SQLite, WAL) is already concurrently readable. |
| Data refresh | Poll the store every ~1s via a `tea.Tick` command | Simple, good enough; swap for notifications later. |

> The one real coupling to add later: a daemon **RPC/IPC** so the TUI can *drive*
> runs (trigger services, deregister) instead of only observing. Until then the TUI
> is read-only + chat. Note it; don't let it gate the first version.

## Architecture (Bubble Tea / Elm)

```
cmd/wxg/tui/
  tui.go        // newTUICmd() cobra command; tea.NewProgram(rootModel{})
  root.go       // rootModel: holds the active tab + sub-models; routes Update/View
  keys.go       // key bindings (tab switch, quit, refresh)
  styles.go     // Lip Gloss styles (palette, borders, focus)
  chat.go       // chatModel  — REPL against the ai package
  runs.go       // runsModel  — live spine: sequences/runs/sessions from the store
  catalog.go    // catalogModel — services + trigger/successor wiring
  store.go      // read-only queries used by runs/catalog (separate from kernel writes)
```

Core loop: `rootModel` implements `Init() / Update(msg) / View()`. It owns a
`tab` enum (Chat | Runs | Catalog) and one sub-model per tab. `Update` forwards
messages to the active sub-model and handles global keys; `View` renders the tab
bar + the active sub-model's view. Each sub-model is itself a Bubble Tea model.

Messages are your own types: `tickMsg` (refresh), `spineMsg{rows}` (query result),
`chatReplyMsg{text}` / `chatErrMsg{err}`. Long work (a query, an AI call) runs in a
`tea.Cmd` (a goroutine returning a `tea.Msg`) — never block `Update`.

## Milestone 1 — shell + chat (the first useful version)

1. `go get github.com/charmbracelet/bubbletea github.com/charmbracelet/lipgloss github.com/charmbracelet/bubbles`.
2. Add `wxg tui` (`newTUICmd`) → `tea.NewProgram(newRoot()).Run()`.
3. `rootModel` with a top tab bar (`Chat` only for now) and `q`/`ctrl+c` to quit.
4. `chatModel`: a `textinput` (prompt) + a `viewport` (scrollback).
   - On Enter: resolve the agent with `ai.ResolveAgent("")` (auto-detect, the same
     code `wxg infer chat` uses) and run `agent.Infer(ctx, ai.Request{Prompt: …})`
     inside a `tea.Cmd`; append the reply (or error) to the scrollback.
   - Show a spinner + `[via codex]` while the call is in flight.
5. Done: you can chat with wuxing's AI interactively. **Ship it.**

## Milestone 2 — live runs (the dashboard)

1. `store.go`: open the same SQLite path read-only
   (`storage.OpenSQLite(path)` is fine — WAL allows concurrent reads while the
   daemon writes). Add queries:
   - recent sequences: `SELECT sequence_id, opened_at, closed_at, outcome FROM wuxing_ft_sequence ORDER BY opened_at DESC LIMIT 50`
   - runs in a sequence (`wuxing_ft_run`), sessions (`wuxing_ft_session`)
   - cost lane: aggregate `ai_ft_call` (tokens, cost) by run/sequence.
2. `runsModel`: a `bubbles/table` of sequences; Enter drills into a sequence's runs
   → sessions → tool facts (the lineage tree). A `tea.Tick` every second re-queries
   so live runs animate (open rows in one color, closed in another).
3. Add the `Runs` tab to the tab bar.

## Milestone 3 — catalog browser

1. Reuse the `library` tool's list/diff (in-process) **or** read the admin index
   (`storage.ListServices`) for the persisted view.
2. `catalogModel`: list services with status (draft/live), and a detail pane
   showing declared triggers + successor wiring (the cascade graph as text).
3. Add the `Catalog` tab.

## Milestone 4 — drive (needs the daemon link)

Once the daemon exposes a small RPC (Unix socket / localhost gRPC / HTTP):
- trigger a service's external trigger from the catalog,
- register/deregister from the catalog,
- tail live events as a push stream instead of polling.

Build the daemon RPC first (a `kernel.Server` over the bus), then wire these.

## Conventions

- **Never block `Update`.** All I/O (store queries, AI calls) returns via `tea.Cmd`.
- **One writer.** The daemon owns writes to the fact store; the TUI only reads.
  Don't assemble a kernel inside the TUI.
- **Style centrally** in `styles.go`; keep a small palette and reuse it.
- **Test the models** headless: call `Update(msg)` with synthetic messages and
  assert on the returned model/state (no terminal needed) — the same way Bubble
  Tea's own tests work.

## Quick reference — the seams you'll plug into

| Need | Use |
|---|---|
| Run a chat/agent turn | `ai.ResolveAgent("")` → `Infer` / `RunAgent` (already used by `wxg infer`) |
| Detected agents (status bar) | `ai.Detect()` |
| Read the spine | `storage.OpenSQLite(path)` + the `wuxing_ft_*` tables (read-only) |
| Persisted catalog | `storage.ListServices(ctx)` |
