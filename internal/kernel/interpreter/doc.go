// Package interpreter is the kernel's central, new face: read cfg → route. It
// recognizes each tool.function call in a service's cfg, validates it against
// the known signature catalog, routes the call over the bus to the owning tool,
// receives the return, feeds it forward, and evaluates successor conditions
// against the emitted fact.
//
// It is gated on the tool function vocabulary (the cfg grammar is derived from
// it) and is stubbed until Phase 5, once ai/connectors publish their signatures
// and the three contracts are locked.
package interpreter
