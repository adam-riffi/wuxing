# wuxing — graph tool

*Orchestration: run a service's internal workflow as a DAG.*

> **Status:** responsibility **settled**, but graph carries **the one unresolved architectural boundary — graph vs the core's interpreter.** That must be settled before its functions are pinned. Mid-level shape below.

## Responsibility (the one job)

Execute a service's **internal** workflow — its DAG across its **own** parts — step by step, awaiting confirmations, with branch nodes routing on a step's emitted value. A single call is a one-step DAG. **Intra-service only:** graph never sequences across services (that's the core's triggers + successor declarations).

## The unresolved boundary (must settle first)

Driving a DAG *is* "interpret and route," which overlaps the core's interpreter. Likely split (russian doll): the **interpreter** parses/validates/routes a single `tool.function` call; **graph** sequences calls into a workflow and invokes the interpreter per step. Until this is fixed, graph's surface is provisional — it may be a thin layer over the interpreter rather than a peer.

## Surface — functions (provisional, pending the boundary)

The shape: advance a workflow to its next node, evaluate a branch condition on a step's output, route to the next step, detect completion. Whether these are *graph's* functions or *the interpreter's* is exactly the open boundary.

## Inputs & outputs (incl. emitted metadata)

In: a service's declared internal workflow (the DAG: nodes = tool/script calls, edges = order, branch nodes = conditions on emitted facts). Out: drives each step, feeds outputs forward, ends at the service's emitted **fact** (`new_set: true`) — which both graph's own branches *and* the service's successor conditions key off. Emits standard tool-call envelope rows per step it drives.

## Config surface

A service's cfg declares its **internal workflow** — the ordered steps, the branch conditions, how a step's output feeds the next. This is the "intra-service workflow format" still open in the system doc (how a service writes its internal DAG).

## Boundaries (what it does *not* do)

No **cross-service** flow (core triggers + successor declarations own that — graph stops at the service boundary), no parsing/validation of the call vocabulary if that lands with the interpreter, no condition-evaluation for *successors* (that's the core, on the emitted fact). Graph routes *within* one service; the cascade between services is not graph's.

## Callers

The core (per service run, to drive its workflow). Not directly a vocabulary a cfg "calls" so much as the engine that *reads* the cfg's workflow section — its caller-relationship is unusual and depends on the boundary resolution.

## Open / deferred

The interpreter-vs-graph boundary (the gating decision); the intra-service workflow notation; the function set, which follows the boundary.
