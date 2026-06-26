// Package facts implements the append-only fact/metric tables — the structured
// event ledger. It owns the lineage spine (wuxing_ft_sequence, _ft_run,
// _ft_session, _ft_scheduler_event, _ft_session_resource, _ft_artifact) and the
// tool-call envelope.
//
// Metadata is append-only: facts are never rewritten, only corrected by filing a
// new row. SQLite-backed; migrations in Phase 4. Per-tool detail tables are
// deferred to each tool's design.
package facts
