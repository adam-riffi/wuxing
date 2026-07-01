// Command wxg is the operator CLI — the control-plane client that reads kernel
// state and issues commands (the `kubectl` of wuxing). At Phase 0 it exposes
// the command tree with stub subcommands; the library/runtime wiring lands in
// later phases.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "wxg:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "wxg",
		Short:         "wuxing operator CLI",
		Long:          "wxg is the control-plane client for the wuxing kernel: read state, manage the service catalog, drive runs.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	root.AddCommand(newLibraryCmd())
	root.AddCommand(newInferCmd())
	root.AddCommand(newRunsCmd())
	root.AddCommand(newShowCmd())
	root.AddCommand(newStateCmd())
	root.AddCommand(newRunCmd())
	return root
}

// newLibraryCmd maps the operator surface of the library tool. The subcommands
// are stubs at Phase 0; they are implemented against the library tool in Phase 5.
func newLibraryCmd() *cobra.Command {
	lib := &cobra.Command{
		Use:   "library",
		Short: "manage the service catalog (index, status, introspection)",
	}

	stub := func(use, short string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return fmt.Errorf("%s: not implemented yet (Phase 5)", cmd.CommandPath())
			},
		}
	}

	lib.AddCommand(
		stub("index <service>", "register a service (draft → live)"),
		stub("deindex <service>", "deregister a service (drain first)"),
		stub("status <service>", "drift: live folder vs canonical snapshot"),
		stub("calls <service>", "introspect declared tool calls"),
		stub("functions <service>", "introspect declared script functions"),
	)
	return lib
}
