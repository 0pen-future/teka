package database

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // migrate driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"teka/apps/api/migrations"
)

// NewMigrator builds a golang-migrate instance over the embedded SQL files.
// Callers own calling m.Close() (via CloseMigrator) when done.
func NewMigrator(databaseURL string) (*migrate.Migrate, error) {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("load embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect migrator: %w", err)
	}
	return m, nil
}

// MigrateUp applies all pending migrations; already-up-to-date is not an error.
func MigrateUp(m *migrate.Migrate) error {
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// MigrateDown rolls back n migrations (all when n <= 0).
func MigrateDown(m *migrate.Migrate, n int) error {
	var err error
	if n <= 0 {
		err = m.Down()
	} else {
		err = m.Steps(-n)
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

// MigrationStatus returns the current schema version and dirty flag.
// version 0 with ok=false means no migration has been applied yet.
func MigrationStatus(m *migrate.Migrate) (version uint, dirty bool, ok bool, err error) {
	version, dirty, err = m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, false, nil
	}
	if err != nil {
		return 0, false, false, fmt.Errorf("migration status: %w", err)
	}
	return version, dirty, true, nil
}
