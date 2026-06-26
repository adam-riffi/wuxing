// Package processors is the derive tool: turn raw emitted signals (connector
// crossings, run/session events, resource samples) into structured metadata and
// metrics — the fact and summary rows the observability layer queries.
//
// It only reduces and records signals already produced; it does not move domain
// data or interpret cfg. Implemented in Phase 5 (minimal: resource-sample rollup).
package processors
