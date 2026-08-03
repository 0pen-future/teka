package cli

import (
	"github.com/spf13/cobra"
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed the database with development data",
	RunE: func(_ *cobra.Command, _ []string) error {
		return notYet("seed")
	},
}
