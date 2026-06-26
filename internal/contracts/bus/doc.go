// Package bus defines the bus message envelope contract — the shape for calls,
// returns, and events, carrying the lineage stamp (sequence_id, run_id, ...).
// This is the gating contract the docs flag as open; it is locked in Phase 2 and
// implemented by the kernel bus in Phase 3.
package bus
