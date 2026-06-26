// Package connectors is the data-I/O tool: move data between a service and an
// external backend through a driver per backend (selected by `type` in cfg),
// using .env credentials. It is also the data-plane meter — every crossing emits
// a movement-fact (audit + volumetry), and a mutating write mints a
// data-mutation event.
//
// Data access is an allowlist: a write is permitted only by an explicit
// DATABASE.TABLE grant, enforced at the boundary. Implemented in Phase 5 (first
// driver: sqlite).
package connectors
