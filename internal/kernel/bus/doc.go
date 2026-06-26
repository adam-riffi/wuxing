// Package bus is the in-process message channel every tool and service talks
// over — synchronous calls (request/return) and asynchronous events. It is part
// of the kernel in the monolith and carries the message envelope (including
// sequence_id) across hops.
//
// Phase 3 implements the Go-channel pub/sub + request/reply transport behind a
// transport abstraction (so an in-container socket variant can land later).
package bus
