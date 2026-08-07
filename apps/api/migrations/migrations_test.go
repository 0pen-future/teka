//go:build integration

package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
)

// domainTables is every table docs/schema_design.sql creates, plus the
// refresh_tokens table from 000002 and zalo_accounts from 000004.
// schema_migrations belongs to golang-migrate itself.
var domainTables = []string{
	"user_accounts", "teachers", "contacts", "students", "classes",
	"class_schedules", "enrollments", "class_sessions", "attendance_records",
	"billing_periods", "invoices", "invoice_lines", "invoice_adjustments",
	"payments", "payment_allocations", "statements", "notifications",
	"refresh_tokens", "zalo_accounts",
}

var views = []string{"v_contact_balance", "v_unbilled_attendance"}

func startBarePostgres(t *testing.T) string {
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
	require.NoError(t, err, "start postgres container (is Docker running?)")
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			t.Logf("terminate postgres container: %v", err)
		}
	})
	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return url
}

func openDB(t *testing.T, url string) *gorm.DB {
	t.Helper()
	db, err := database.Open(config.DatabaseConfig{
		URL:             url,
		MaxOpenConns:    2,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close(db) })
	return db
}

func nameSet(t *testing.T, db *gorm.DB, query string) map[string]bool {
	t.Helper()
	var rows []string
	require.NoError(t, db.Raw(query).Scan(&rows).Error)
	names := map[string]bool{}
	for _, n := range rows {
		names[n] = true
	}
	return names
}

func tableNames(t *testing.T, db *gorm.DB) map[string]bool {
	return nameSet(t, db,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`)
}

func viewNames(t *testing.T, db *gorm.DB) map[string]bool {
	return nameSet(t, db,
		`SELECT table_name FROM information_schema.views WHERE table_schema = 'public'`)
}

func TestMigrationRoundTrip(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })

	db := openDB(t, url)

	// Up: every domain table, refresh_tokens, both views.
	require.NoError(t, database.MigrateUp(m))
	tables := tableNames(t, db)
	for _, want := range domainTables {
		require.Truef(t, tables[want], "table %s missing after migrate up", want)
	}
	require.True(t, tables["schema_migrations"], "schema_migrations missing")
	vs := viewNames(t, db)
	for _, want := range views {
		require.Truef(t, vs[want], "view %s missing after migrate up", want)
	}

	// Down to zero: only schema_migrations remains, no views.
	require.NoError(t, database.MigrateDown(m, 0))
	tables = tableNames(t, db)
	require.Len(t, tables, 1, "want only schema_migrations after full down, got %v", tables)
	require.True(t, tables["schema_migrations"])
	require.Empty(t, viewNames(t, db))

	// Up again: the round trip must be clean.
	require.NoError(t, database.MigrateUp(m))
	tables = tableNames(t, db)
	for _, want := range domainTables {
		require.Truef(t, tables[want], "table %s missing after re-up", want)
	}
}

func TestRefreshTokensFKTargetsUserAccounts(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	var row struct {
		TableName  string
		DeleteRule string
	}
	require.NoError(t, db.Raw(`
		SELECT ccu.table_name, rc.delete_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu
		  ON ccu.constraint_name = tc.constraint_name
		JOIN information_schema.referential_constraints rc
		  ON rc.constraint_name = tc.constraint_name
		WHERE tc.table_name = 'refresh_tokens' AND tc.constraint_type = 'FOREIGN KEY'`,
	).Scan(&row).Error)
	target, deleteRule := row.TableName, row.DeleteRule
	require.Equal(t, "user_accounts", target)
	require.Equal(t, "CASCADE", deleteRule)
}
