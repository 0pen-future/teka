// Package cli defines the Cobra command tree for the API binary.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "api",
	Short:         "Teka API server and operations CLI",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(serveCmd, migrateCmd, seedCmd, adminCmd)
}

// Execute runs the root command and exits non-zero on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// notYet marks commands whose implementation lands in a later provisioning
// phase; they fail loudly instead of pretending to succeed.
func notYet(what string) error {
	return fmt.Errorf("%s is provisioned in a later phase (see plans/)", what)
}
