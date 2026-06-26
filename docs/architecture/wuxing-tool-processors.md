# wuxing — processors tool

*Derive: turn raw signals into metadata and metrics.*

> **Status:** the **least-designed** tool — responsibility and boundaries are clear, but it's a ride-along that consumes what the others emit, so its detail follows the rest of the data model. Thin by nature and low-volume. Mid-level shape only.

## Responsibility (the one job)

Take the raw signals the platform produces (connector crossings, run/session events, resource samples) and **derive** structured metadata and metrics from them — building the fact tables and summary rows the observability layer queries. It computes *records about* what happened; it does not produce or move domain data.

## Surface — functions (provisional)

The shape: ingest raw emitted signals, compute derived facts/metrics (e.g. summarize a live resource series into a peak/avg row, roll crossings into volumetry), write the derived rows. Precise functions follow the metadata schema, which is partly deferred to per-tool data-model design.

## Inputs & outputs (incl. emitted metadata)

In: raw signals from the other tools and the kernel (crossings, scheduler decisions, resource samples, outcomes). Out: derived **fact** and **summary** rows in the structured tables. Processors are themselves low-activity — they run to reduce and record, not to act. They are the producer side of much of the **tracking** tier, applying the "capture at the source, decide later" / "summary grain, kept indefinitely" rules.

## Config surface

Minimal — processors are mostly platform machinery, not service-configured. What's configurable is retention/summarization policy (what to sample, what to persist, what to discard) rather than per-service behaviour.

## Boundaries (what it does *not* do)

Does not move data across the boundary (connectors), does not store **domain** payloads (only derived metadata), does not interpret cfg or drive workflows (core/graph), does not infer (ai). It only **reduces and records** signals already produced — the data model's faithful-recorder mechanics.

## Callers

The core/the platform pipeline (signals flow to processors as events occur). Rarely if ever a cfg vocabulary verb — like the library, it's defined by what feeds it, not by words it adds to the cfg language.

## Open / deferred

Most of its detail is downstream of the **deferred per-tool data model** (its outputs *are* those tables). Finalize after the metadata schema firms up per tool.
