// Package artifacts is the artifact store: services' actual flat-file outputs
// (md, json, drafts) live on a filesystem and are referenced by the tables,
// never ingested — wuxing records path + metadata only. Distinct from the
// per-agent scratch space, which is swept each run. Implemented in Phase 4.
package artifacts
