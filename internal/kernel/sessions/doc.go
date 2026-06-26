// Package sessions is the kernel's running registry: it tracks live instances,
// what is running, and their resource profiles. It answers the drain check the
// library's deregister consults — a service cannot be reindexed while it has
// running instances.
//
// Implemented in Phase 3.
package sessions
