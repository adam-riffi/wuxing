package integration

import "testing"

// TestPlaceholder keeps the integration package compilable and the CI job green
// until real integration targets exist (Phase 3). It is skipped in short mode.
func TestPlaceholder(t *testing.T) {
	if testing.Short() {
		t.Skip("integration tests skipped in -short mode")
	}
	t.Skip("no integration targets yet (Phase 3)")
}
