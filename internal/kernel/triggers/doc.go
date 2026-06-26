// Package triggers watches for initiation and fires runs: cron schedules and
// bus-event rules (including successor declarations emitted by finished
// services). It propagates sequence_id across event hops — an external trigger
// opens a new sequence, an event trigger inherits the parent's.
//
// This inter-service event coupling is the only mechanism that bridges services;
// intra-service flow is the graph tool's job. Implemented in Phase 3.
package triggers
