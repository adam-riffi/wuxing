// Package integration holds cross-component tests that require real
// infrastructure (a Docker daemon, real SQLite files). They are excluded from
// the short unit run and gated to the linux CI job. Targets land from Phase 3.
package integration
