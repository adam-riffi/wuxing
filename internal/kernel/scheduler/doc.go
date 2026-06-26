// Package scheduler is the kernel's dual-resource admission control. It is the
// only thing that sees total resources, every session, and the queue.
//
// Two finite resources with opposite refill behaviour: memory (frees on
// completion) and the AI quota window (refills on a clock). A job runs only when
// its full request fits — never partial. Order is priority-class then FCFS, with
// backfill; patience (max_wait) drives escalate/overclock/fail on expiry.
//
// Implemented in Phase 3.
package scheduler
