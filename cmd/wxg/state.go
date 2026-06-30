package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/adam-riffi/wuxing/internal/control"
)

// newStateCmd shows the running daemon's live state — resources, queue, and
// running sessions — by reading its control API. This is live (in-memory) state,
// distinct from the historical fact store that `wxg runs`/`show` read.
func newStateCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "state",
		Short: "live daemon state: resources, queue, running sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Get("http://" + addr + "/state")
			if err != nil {
				return fmt.Errorf("cannot reach the daemon at %s — is `wuxing` running? (%w)", addr, err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("daemon returned %s", resp.Status)
			}
			var st control.State
			if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
				return fmt.Errorf("decode state: %w", err)
			}

			ew := &errWriter{w: cmd.OutOrStdout()}
			ew.printf("resources\n")
			ew.printf("  memory     %s used / %s  (%s free)\n",
				humanBytes(st.Memory.Used), humanBytes(st.Memory.Capacity), humanBytes(st.Memory.Free))
			if st.Window.Capacity > 0 {
				ew.printf("  ai window  %d used / %d  (%d free)\n", st.Window.Used, st.Window.Capacity, st.Window.Free)
			} else {
				ew.printf("  ai window  disabled\n")
			}

			ew.printf("\nrunning jobs (%d)\n", len(st.RunningJobs))
			for _, id := range st.RunningJobs {
				ew.printf("  %s\n", short(id))
			}
			ew.printf("queued jobs (%d)\n", len(st.Queue))
			for _, id := range st.Queue {
				ew.printf("  %s\n", short(id))
			}
			ew.printf("running sessions (%d)\n", len(st.Sessions))
			for _, s := range st.Sessions {
				ew.printf("  %s  service=%s  run=%s\n", short(s.Session), s.Service, short(s.Run))
			}
			return ew.err
		},
	}
	cmd.Flags().StringVar(&addr, "api", control.DefaultAddr, "daemon control API address")
	return cmd
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
