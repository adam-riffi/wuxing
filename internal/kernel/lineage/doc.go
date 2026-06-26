// Package lineage is the kernel's bureaucrat: it assigns all metadata — every id
// (sequence/run/session/call/tool_function) and order field — and writes the
// spine tables. The observed never authors the observation; lineage stamps from
// the outside so the record cannot be forged.
//
// Order is explicit (integer rank), never inferred from clocks. Implemented in
// Phase 3, writing to the storage spine from Phase 4.
package lineage
