# wuxing — the core (kernel)

*Not a tool — the substrate every tool and service plugs into. The one irreducible part that can't sit behind the bus.*

> **Status:** mostly designed in principle (system doc), with **one face genuinely new and central — the interpreter** — and **one unresolved boundary** (interpreter vs graph). The interpreter is **gated on the tool vocabulary**: it validates `tool.function` calls against known signatures and parses a grammar *derived from* that vocabulary, so it can't be finalized until ai/connectors are catalogued. The other faces are detailable now.

## What the core is

A long-running, compiled daemon. Its defining verb is **read cfg → route**. Everything that must see the whole picture — total resources, every running instance, the queue, the event stream, the lineage — lives here, because it can't be delegated to something reached *over* the bus. "Brain barely decides; hands do": the core decides *where work goes*, the work happens on the far side.

## The seven faces

**1 — The interpreter (the new, central work).** Reads a service's cfg; recognizes each `tool.function` call; validates it against the known signature catalog; routes the call over the bus to the owning tool; receives the return and feeds it forward; evaluates successor conditions against the emitted fact. This *is* "read cfg and route," and it's the face the library refused to be. Finalization waits on the vocabulary + the derived grammar.

**2 — The bus.** The in-process message channel (part of the kernel in the monolith) every tool and service talks over — synchronous calls (request/return) and asynchronous events. *Open:* the message envelope shape (a gating contract), and the transport by which a service **in a container** reaches the bus **in the daemon** (socket/IPC).

**3 — The scheduler.** Dual-resource admission: memory (frees on completion) + AI window (clock-refilled), request/limit envelopes, full-request admission, queueing as back-pressure, priority class + FCFS + backfill, patience + escalate/overclock. *Designed in principle; open at the algorithm/data-structure level, plus measuring real memory and tracking the window via Codex `--json`.*

**4 — Sessions & running registry.** Tracks live instances, what's running, and their resource profiles (peak/avg, sampled live then summarized). Answers the **drain check** the library's `deregister` consults. *Open:* the sampling mechanism and its cost.

**5 — Triggers.** Watches for initiation — cron/external events, and **successor declarations** emitted by finished services — fires runs, **propagates `sequence_id`** across event hops, opens a sequence on an external trigger, closes it when a completion fires no further run. *Designed and refined; open: the watching mechanism and where condition-evaluation runs.*

**6 — The launcher.** Drives the container engine (Docker) to run a service's image with its envelope; provisions the scratch space; injects `.env`; wires the container to the bus (couples with face 2). *Open:* warm vs cold containers; image build at index time (couples with the library).

**7 — Lineage stamping.** Assigns **all** metadata — every id (`sequence/run/session/call/tool_function`) and order field — and writes the spine tables. The bureaucrat. *Designed as the data-model spine; open: where each stamp happens in the flow.*

## The one unresolved boundary — interpreter vs graph

Driving a workflow DAG *is* "interpret and route," which overlaps graph. The likely split (russian doll): the **interpreter** parses/validates/routes a *single* call; **graph** sequences calls into a service's internal workflow and calls the interpreter per step. This must be settled when the interpreter is finalized — it shapes both.

## The three contracts the core owns (gate everything)

- **cfg schema** — the structural grammar; *derived from the vocabulary*, so it follows the tool catalogs.
- **SDK / script contract** — how a service's script is entered, and how it reports its fact / emits to the bus.
- **bus message shape** — the envelope for calls, returns, and events (carrying `sequence_id`).

## Boundaries (what the core does *not* do)

It does not *do the work* (services do), does not move data (connectors), does not infer (ai), does not persist definitions (library — the core *consults* it), does not run containers itself (drives the engine). It routes, schedules, watches, launches, and records — nothing domain-specific.

## Critical path

**vocabulary (ai, connectors) → derive cfg grammar → lock the three contracts → finalize the interpreter (+ settle the graph boundary) → scaffold → build.** The mechanical faces (scheduler/sessions/triggers/launcher/bus/lineage) are detailed alongside, since they're settled in principle. The headline: the core is mostly designed *except the interpreter*, and the interpreter is gated on the vocabulary — so the next real work is the heavy tools' function catalogs.
