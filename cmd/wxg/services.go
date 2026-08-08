package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// newServicesCmd lists registered services from the admin index — the wuxing
// equivalent of `kubectl get deployments`. It reads the fact store directly, so
// it works without a running daemon.
func newServicesCmd() *cobra.Command {
	var store string
	cmd := &cobra.Command{
		Use:   "services",
		Short: "list registered services (from the admin index)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openStore(store)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			recs, err := db.ListServices(cmd.Context())
			if err != nil {
				return fmt.Errorf("list services: %w", err)
			}
			if len(recs) == 0 {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "no services registered yet")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			ew := &errWriter{w: tw}
			ew.printf("NAME\tSTATUS\tVERSION\tREGISTERED_AT\n")
			for _, r := range recs {
				ew.printf("%s\t%s\t%s\t%s\n", r.Name, r.Status, r.Version, r.RegisteredAt)
			}
			if ew.err != nil {
				return ew.err
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&store, "store", defaultStore, "path to the fact store")
	return cmd
}
