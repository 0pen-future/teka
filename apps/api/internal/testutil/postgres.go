// Package testutil provides integration-test infrastructure: a migrated
// PostgreSQL testcontainer, a fully wired test server, and fixture builders.
// Everything here is test-only; production code never imports it.
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
)

// StartPostgres launches a disposable PostgreSQL container, applies all
// embedded migrations, and returns a connected *gorm.DB. Each call gets its
// own container, so tests and packages are fully isolated and parallel-safe;
// the container is terminated via t.Cleanup. Skipped under -short so the fast
// unit loop never needs Docker.
func StartPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker, skipped with -short")
	}

	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("teka_test"),
		tcpostgres.WithUsername("teka"),
		tcpostgres.WithPassword("teka"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container (is Docker running?): %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			t.Logf("terminate postgres container: %v", err)
		}
	})

	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("container connection string: %v", err)
	}

	m, err := database.NewMigrator(url)
	if err != nil {
		t.Fatalf("build migrator: %v", err)
	}
	if err := database.MigrateUp(m); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
		t.Fatalf("close migrator: source=%v db=%v", srcErr, dbErr)
	}

	db, err := database.Open(config.DatabaseConfig{
		URL:             url,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("connect gorm: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	return db
}
