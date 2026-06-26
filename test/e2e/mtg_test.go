package e2e

import "testing"

// TestMTGNewSetNotifier is the M1 acceptance test: the full MTG cascade
// (checker → connector write → event trigger → notifier → messenger) observed in
// the tracking tables under a single sequence_id. Implemented in Phase 7.
func TestMTGNewSetNotifier(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e tests skipped in -short mode")
	}
	t.Skip("MTG end-to-end is the Phase 7 / M1 acceptance target")
}
