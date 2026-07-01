# The cfg — anatomy of a service

**The cfg is the program.** A service is a *folder* under `services/`: one cfg
file (`cfg.yml`) plus, when needed, its scripts (Python, shell, SQL, JS). The
cfg is the service's **ID card** — the interpreter reads it and routes; nothing
about the service lives anywhere else. The daemon loads the folder at boot:

```bash
wuxing --services ./services
```

```
services/
  mtg/                    # a service is a folder
    cfg.yml               #   its ID card (the program)
    scripts/              #   its own code, when yml alone isn't enough
      check_scryfall.py
      scrape.py
  notifier/
    cfg.yml               # yml-only service: no scripts at all
```

## The sections of a cfg

| Section | What it declares | Read by |
|---|---|---|
| `name` / `version` | identity (folder name is the default) | library, admin index |
| `envelope` | resource profile: memory floor/ceiling, AI-window cost, priority, queue patience | scheduler (admission) |
| `allow` | the capability allowlist — every tool.operation (and target) the service may call; anything else is refused at the bus | connectors/ai (grants) |
| `triggers` | when the service starts: `cron` (clock) or `event` (a topic another service emitted) | triggers face |
| `workflow` | the internal route: ordered steps with branching — **the program body** | interpreter |
| `successors` | what runs after this service emits its fact — the *only* inter-service coupling, indirect and one-way | triggers face |

## A yml-only service (runs today)

Pure tool-composition needs no script — the interpreter routes every step over
the bus. This is [`services/mtg/cfg.yml`](../services/mtg/cfg.yml), abridged:

```yaml
name: mtg
envelope:
  request: 67108864            # 64 MiB admission floor
  ai_request: 1                # costs 1 unit of the AI window
  priority: background
allow:
  - { tool: ai, operation: infer }
  - { tool: connectors, operation: write, target: MTG.SETS }
triggers:
  - { kind: cron, spec: "0 9 * * *" }
workflow:
  - id: check
    tool: ai
    operation: infer
    with: { prompt: "Did a new MTG set release today? Reply as JSON {new_set,set_name}." }
    branch:
      - { when: new_set == true, goto: record }
  - id: record
    tool: connectors
    operation: write
    with: { target: MTG.SETS }
successors:
  - { service: notifier, topic: mtg-db updated, when: new_set == true }
```

How it executes: cron fires → the scheduler admits it under the envelope → the
interpreter walks the workflow (`check` → branch on the emitted `new_set` →
`record`) → each step's output merges into the accumulated **fact** → on
completion the successor condition is evaluated on that fact → `notifier` runs
under the **same sequence** (one causal chain in the spine).

## A service with Python scripts

A **script step** replaces `tool`/`operation` with `script:` — a path inside the
service folder. Scripts exist for **bespoke logic** (the long tail of external
APIs: Python clients, raw HTTP, scraping) that no platform tool covers:

```yaml
name: mtg
envelope: { request: 67108864, priority: background }
allow:
  - { tool: connectors, operation: write, target: MTG.SETS }
triggers:
  - { kind: cron, spec: "0 9 * * *" }
workflow:
  - id: check
    script: scripts/check_scryfall.py     # the service's own code
    branch:
      - { when: new_set == true, goto: scrape }

  - id: scrape
    script: scripts/scrape.py             # feeds on check's emitted fact
    next: record

  - id: record
    tool: connectors                      # tool + script steps mix freely
    operation: write
    with: { target: MTG.SETS }
successors:
  - { service: notifier, topic: mtg-db updated, when: new_set == true }
```

```
services/mtg/
  cfg.yml
  scripts/
    check_scryfall.py
    scrape.py
  requirements.txt        # the service's own deps — sealed into its image
```

### The script contract (the SDK)

A script is a **sealed job**: it runs inside the service's own container, gets
its inputs, does its work, and **reports a fact**. The contract:

- **Input:** the accumulated fact so far, merged with the step's `with:` args,
  as **JSON on stdin**. Secrets arrive as env vars (the cfg references
  `${SCRYFALL_KEY}`; `.env` holds the value; the launcher injects it).
- **Output:** the script's **emitted fact as JSON on stdout** — e.g.
  `{"new_set": true, "set_name": "Bloomburrow"}`. That fact is what branches
  route on, what successor conditions evaluate, and what feeds the next step.
- **Outcome:** the exit code. `0` = success; non-zero maps to wuxing's fixed
  outcome vocabulary (the service *classifies* domain errors — an API rate-limit,
  a DB code — into that vocabulary; wuxing *controls* what happens next:
  stop / retry / standby per the cfg's declared policy).
- **Workspace:** an ephemeral scratch dir (dies with the run) plus a small
  durable checkpoint (the idempotency high-water mark, written only on success).

```python
# scripts/check_scryfall.py — a conforming script
import json, os, sys, urllib.request

inputs = json.load(sys.stdin)                     # accumulated fact + with-args
key = os.environ["SCRYFALL_KEY"]                  # injected from .env

sets = fetch_recent_sets(key)                     # the bespoke logic
latest = sets[0]["name"]
seen = read_checkpoint()                          # durable high-water mark

json.dump({"new_set": latest != seen, "set_name": latest}, sys.stdout)
sys.exit(0)                                       # outcome: success
```

**Isolation rules:** a script sees *only* its own folder, its scratch dir, and
its injected env. It never imports from another service (self-contained to the
point of duplication), never calls another service (it emits an event, blind),
and reaches tools only through declared workflow steps — a script that wants to
write the DB doesn't open a connection; the *next step* is a `connectors.write`
under the `allow` grant.

### Declarative vs scripted — when to use which

| | yml-only | with scripts |
|---|---|---|
| logic | composing platform tools (ai, connectors) | bespoke code: API clients, scraping, parsing |
| runs | **in-process, today** | in the service's **container** (needs the Docker engine) |
| trajectory | the end state — services become declarative enough to need no script | the explicit interim; the "universal connector" north star absorbs API reach into cfg |

## Status — what's implemented

| Piece | State |
|---|---|
| cfg schema: envelope, allow, triggers, workflow (tool steps, `with:`, branch/next), successors | ✅ parsed + validated |
| `script:` steps in the schema | ✅ parsed + validated (path hygiene, script XOR tool) |
| Daemon loading (`--services` dir → register + admin index) | ✅ |
| yml-only execution (interpreter over the bus) | ✅ |
| **Script execution** (launcher → container → stdin/stdout contract) | ❌ needs the real Docker engine; a script step fails with a clear "container engine not wired" error today |
| Two-tier error policy (retry/standby), `${VAR}` secret expansion, checkpoint store | ❌ designed, not built |
