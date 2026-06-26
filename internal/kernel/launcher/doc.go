// Package launcher drives the container engine (Docker) to run a service's image
// with its resource envelope: it provisions the scratch space, injects .env, and
// wires the container to the bus.
//
// Implemented in Phase 3 (integration-tested against a real Docker daemon).
package launcher
