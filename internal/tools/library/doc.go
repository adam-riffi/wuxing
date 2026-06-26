// Package library is the catalog tool: persist and serve service definitions. A
// registry, not a runtime — it never interprets, launches, or evaluates.
//
// Writes (lifecycle): register, deregister. Reads (serve): get_definition,
// get_successors, get_triggers, diff, list, read_declarations. The operator
// surface (wxg library index/deindex/status/calls/functions) maps over a subset.
//
// Implemented in Phase 5.
package library
