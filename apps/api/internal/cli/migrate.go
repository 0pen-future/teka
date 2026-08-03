package cli

import (
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database schema migrations",
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	RunE: func(_ *cobra.Command, _ []string) error {
		return notYet("migrate up")
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Roll back migrations",
	RunE: func(_ *cobra.Command, _ []string) error {
		return notYet("migrate down")
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current migration version",
	RunE: func(_ *cobra.Command, _ []string) error {
		return notYet("migrate status")
	},
}

func init() {
	migrateCmd.AddCommand(migrateUpCmd, migrateDownCmd, migrateStatusCmd)
}
