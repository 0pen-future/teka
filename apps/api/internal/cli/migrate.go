package cli

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/spf13/cobra"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
)

var (
	migrateDownSteps int
	migrateDownAll   bool
	migrateDownYes   bool
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database schema migrations",
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return withMigrator(func(m *migrate.Migrate) error {
			if err := database.MigrateUp(m); err != nil {
				return err
			}
			cmd.Println("migrations applied")
			return nil
		})
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Roll back migrations (default: one step)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		steps := migrateDownSteps
		if migrateDownAll {
			// A full rollback drops every table; outside development it must
			// be explicitly confirmed.
			if !cfg.IsDevelopment() && !migrateDownYes {
				return errors.New("refusing full rollback outside development; re-run with --yes to confirm")
			}
			steps = 0
		} else if steps <= 0 {
			return errors.New("--steps must be at least 1 (use --all for a full rollback)")
		}

		m, err := database.NewMigrator(cfg.Database.URL)
		if err != nil {
			return err
		}
		defer closeMigrator(m)
		if err := database.MigrateDown(m, steps); err != nil {
			return err
		}
		cmd.Println("rollback complete")
		return nil
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current migration version",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return withMigrator(func(m *migrate.Migrate) error {
			version, dirty, ok, err := database.MigrationStatus(m)
			if err != nil {
				return err
			}
			if !ok {
				cmd.Println("no migrations applied")
				return nil
			}
			cmd.Printf("version: %d dirty: %v\n", version, dirty)
			return nil
		})
	},
}

// withMigrator loads config, builds a migrator, runs fn, and cleans up.
func withMigrator(fn func(m *migrate.Migrate) error) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	m, err := database.NewMigrator(cfg.Database.URL)
	if err != nil {
		return err
	}
	defer closeMigrator(m)
	return fn(m)
}

func closeMigrator(m *migrate.Migrate) {
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		fmt.Println("warning: closing migration source:", srcErr)
	}
	if dbErr != nil {
		fmt.Println("warning: closing migration database:", dbErr)
	}
}

func init() {
	migrateDownCmd.Flags().IntVar(&migrateDownSteps, "steps", 1, "number of migrations to roll back")
	migrateDownCmd.Flags().BoolVar(&migrateDownAll, "all", false, "roll back all migrations")
	migrateDownCmd.Flags().BoolVar(&migrateDownYes, "yes", false, "confirm full rollback outside development")
	migrateCmd.AddCommand(migrateUpCmd, migrateDownCmd, migrateStatusCmd)
}
