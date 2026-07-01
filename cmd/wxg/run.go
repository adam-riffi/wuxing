package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/adam-riffi/wuxing/internal/control"
)

// newRunCmd fires a service on demand through the running daemon — a manual
// external trigger opening a new sequence. The minimal form of the run-control
// design (docs/cli-run-control.md); targeting/inputs/sequencing flags extend it.
func newRunCmd() *cobra.Command {
	var addr string
	var timeout int
	cmd := &cobra.Command{
		Use:   "run <service>",
		Short: "fire a service now (manual trigger, new sequence)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, _ := json.Marshal(control.RunRequest{Service: args[0]})
			client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
			resp, err := client.Post("http://"+addr+"/run", "application/json", bytes.NewReader(body))
			if err != nil {
				return fmt.Errorf("cannot reach the daemon at %s — is `wuxing` running? (%w)", addr, err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				var apiErr struct {
					Error string `json:"error"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&apiErr)
				if apiErr.Error == "" {
					apiErr.Error = resp.Status
				}
				return fmt.Errorf("daemon refused the run: %s", apiErr.Error)
			}

			var started control.RunStarted
			if err := json.NewDecoder(resp.Body).Decode(&started); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
			ew := &errWriter{w: cmd.OutOrStdout()}
			ew.printf("fired %s\n", started.Service)
			ew.printf("  sequence %s\n", started.Sequence)
			ew.printf("  run      %s\n", started.Run)
			ew.printf("follow it: wxg show %s\n", short(started.Sequence))
			return ew.err
		},
	}
	cmd.Flags().StringVar(&addr, "api", control.DefaultAddr, "daemon control API address")
	cmd.Flags().IntVar(&timeout, "timeout", 600, "client timeout in seconds (runs execute synchronously)")
	return cmd
}
