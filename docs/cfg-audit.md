# cfg parameter audit — what a control plane's service contract needs

An honest inventory of the cfg surface: what is declared **and enforced**, what
is declared **but ignored**, and what is **missing** for a scheduler-governed,
container-running, AI-budgeted control plane. Kept current as gaps close.

Three tiers. The dangerous one is the middle: a parameter that parses fine and
does nothing teaches the operator false confidence.

## Tier 1 — declared and enforced

| Param | Where | Enforced by |
|---|---|---|
| `name` / `version` | root | library identity, admin index |
| `envelope.request` | envelope | scheduler admission (memory floor) |
| `envelope.priority` | envelope | scheduler class (user ahead of background) |
| `envelope.ai_request` | envelope | AI-window admission (dual-resource) |
| `envelope.max_wait` | envelope | queue patience before the starve policy fires |
| `envelope.on_starve` | envelope | `escalate` (age into the privileged class → may draw the overclock reserve) or `fail` (drop + record) |
| `allow` (tool/operation/target grants) | root | connectors grant check at the bus (the daemon grants the union of loaded allowlists) |
| `triggers` (cron/event) | root | trigger registration + the daemon's cron watcher (fires due schedules as new sequences) |
| trigger `timezone` | triggers | folded into the cron schedule (`CRON_TZ`), validated at load |
| `concurrency` (allow/forbid) | root | `forbid` skips a fire while a run of the service is in flight (k8s CronJob "Forbid" semantics) |
| `workflow` steps: tool XOR script, `with:`, `next`/`branch` | workflow | interpreter (script *execution* pending Docker) |
| `successors` (service/topic/when) | root | condition eval + cascade firing |

## Tier 2 — declared but NOT yet enforced (parses, then ignored)

| Param | Parsed | What ignores it | Becomes real when |
|---|---|---|---|
| `envelope.limit` (burst ceiling) | ✅ | threaded to the scheduler job, but the *enforcer* is the container's memory cap | the Docker engine lands (launcher applies it at launch) |
| `allow` grants for `ai.*` | ✅ | the ai tool doesn't consult grants (connectors does) | ai grant check added |

## Tier 3 — missing from the schema entirely

Grouped by the concern that needs them; ordered by how soon each bites.

### Execution (bites at Docker time)
| Missing param | Why it's needed | Prior art |
|---|---|---|
| `image` | the service's sealed container — the launcher's `ServiceSpec.Image` already requires it; today nothing declares it | k8s `image` |
| `env` (secret *references*) | `${SCRYFALL_KEY}` refs the cfg promises exist have no section to live in; launcher injects env | 12-factor, compose `environment` |
| per-step `timeout` | a hung script/tool call must not hold memory + a session forever; MaxWait bounds *queueing*, nothing bounds *running* | k8s `activeDeadlineSeconds` |
| step `retry` / `backoff` | the designed two-tier error policy (stop / retry / standby) has no declaration | k8s `backoffLimit`, Airflow `retries` |
| `on_error` (run-level policy) | stop / retry / standby per the design's fixed outcome vocabulary | supervisor policies |

### Scheduling
| Missing param | Why |
|---|---|
| `concurrency: replace` | allow + forbid exist; "replace" (cancel the running one, start fresh) needs run cancellation first |
| trigger `debounce` / coalescing | a chatty event topic shouldn't fan out into N queued duplicate runs |

### AI governance (bites with real backends)
| Missing param | Why |
|---|---|
| `ai.model` / `ai.backend` preference | today the backend is global (auto-detect); a service may need a specific model/CLI |
| `ai.max_cost` per run | the window bounds *rate*; nothing bounds *spend* per run — the cost fact records, nothing enforces |
| `ai.max_turns` for agent mode | a bounded content producer should declare its bound |

### Resources beyond memory (later, honest to name)
| Missing param | Why |
|---|---|
| `cpu` shares/limit | memory-only envelope; a busy scraper can starve the host CPU |
| `disk` (scratch quota) | the scratch dir is unbounded today |
| network egress policy | the sealed-box story ultimately wants an outbound allowlist per service |

### Operational metadata (cheap, anytime)
| Missing param | Why |
|---|---|
| `description` / `owner` / `tags` | catalog hygiene once services multiply |
| `enabled: false` | park a service without deleting its folder |

## The starvation story, end to end (now declarable)

> *"How long can it stay starved before asking for overclocking?"*

```yaml
envelope:
  request: 67108864     # admission floor: don't start until 64 MiB is free
  limit: 134217728      # burst ceiling: the container is capped at 128 MiB
  ai_request: 1         # also needs 1 unit of the AI window to be admitted
  priority: background  # admitted behind user-class work
  max_wait: 10m         # tolerate queueing for 10 minutes…
  on_starve: escalate   # …then age up to user class — which may draw the
                        # overclock reserve band (the fenced-off memory normal
                        # background work can't touch)
```

`on_starve: fail` is the alternative: past `max_wait`, drop the run and record
the outcome instead of escalating.

## Recommended sequencing

1. ~~Thread the envelope into the scheduler job + `on_starve`~~ — **done**.
2. ~~With the cron loop: `timezone`, `concurrency`~~ — **done** (replace-policy
   and debounce remain).
3. **With the Docker engine:** `image`, `env`, per-step `timeout`, `retry`,
   `on_error` — the execution contract.
4. **With real AI backends:** the `ai.*` governance block.
5. **Backlog:** cpu/disk/egress, metadata block.
