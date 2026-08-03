package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"teka/apps/api/internal/app"
	"teka/apps/api/internal/config"
	"teka/apps/api/seeds"
)

var seedForce bool

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed the database with development data",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		// Seed data carries well-known dev credentials; production needs an
		// explicit override.
		if cfg.IsProduction() && !seedForce {
			return errors.New("refusing to seed a production database; re-run with --force to override")
		}

		c, err := app.NewContainer(cfg)
		if err != nil {
			return err
		}
		defer c.Close()
		return seeds.Run(cmd.Context(), c.DB, c.Log)
	},
}

func init() {
	seedCmd.Flags().BoolVar(&seedForce, "force", false, "allow seeding in production")
}
