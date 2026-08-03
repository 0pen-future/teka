package cli

import (
	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Administrative account operations",
}

var adminCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an administrator account",
	RunE: func(_ *cobra.Command, _ []string) error {
		return notYet("admin create")
	},
}

func init() {
	adminCmd.AddCommand(adminCreateCmd)
}
