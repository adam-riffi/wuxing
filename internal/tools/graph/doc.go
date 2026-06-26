// Package graph is the orchestration tool: execute a service's internal workflow
// — its DAG across its own parts — step by step, branching on a step's emitted
// value. Intra-service only; the cascade between services is the kernel's
// triggers, not graph.
//
// Graph carries the one unresolved architectural boundary — graph vs the
// interpreter (the likely russian-doll split: graph sequences calls and invokes
// the interpreter per step). It stays a stub until that boundary is settled in
// Phase 6.
package graph
