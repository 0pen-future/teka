//go:build integration

package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
)

// domainTables is every table docs/schema_design.sql creates, plus the
// refresh_tokens table from 000002, zalo_accounts from 000004 and
// notification_runs from 000005. schema_migrations belongs to golang-migrate
// itself.
var domainTables = []string{
	"user_accounts", "teachers", "contacts", "students", "classes",
	"class_schedules", "enrollments", "class_sessions", "attendance_records",
	"billing_periods", "invoices", "invoice_lines", "invoice_adjustments",
	"payments", "payment_allocations", "statements", "notifications",
	"refresh_tokens", "zalo_accounts", "notification_runs",
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

// The zalo_personal channel and the contact mapping columns arrive together:
// a notification row may carry the new channel value, and contacts carry the
// Zalo identity the picker mapped.
func TestZaloPersonalMappingSchema(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)

	contactCols := nameSet(t, db,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'contacts'`)
	require.True(t, contactCols["zalo_user_id"], "contacts.zalo_user_id missing")
	require.True(t, contactCols["zalo_name"], "contacts.zalo_name missing")

	notifCols := nameSet(t, db,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'notifications'`)
	require.True(t, notifCols["run_id"], "notifications.run_id missing")

	var channelCheck string
	require.NoError(t, db.Raw(
		`SELECT pg_get_constraintdef(oid) FROM pg_constraint
		 WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_channel_check'`,
	).Scan(&channelCheck).Error)
	require.Contains(t, channelCheck, "zalo_personal",
		"notifications channel CHECK must allow zalo_personal")

	var runStatusCheck string
	require.NoError(t, db.Raw(
		`SELECT pg_get_constraintdef(oid) FROM pg_constraint
		 WHERE conrelid = 'notification_runs'::regclass AND contype = 'c'
		   AND pg_get_constraintdef(oid) LIKE '%status%'`,
	).Scan(&runStatusCheck).Error)
	for _, status := range []string{"running", "completed", "interrupted", "expired"} {
		require.Contains(t, runStatusCheck, status,
			"notification_runs status CHECK must allow %s", status)
	}
}

// notificationFixture is the minimal parent chain a notifications row needs:
// teacher, contact, billing period, statement.
type notificationFixture struct {
	teacherID   uuid.UUID
	contactID   uuid.UUID
	periodID    uuid.UUID
	statementID uuid.UUID
}

// seedNotificationParents inserts one teacher with the full chain down to a
// statement, so tests can attach notifications and runs to real rows. phone
// must be unique per call within one database.
func seedNotificationParents(t *testing.T, db *gorm.DB, phone string) notificationFixture {
	t.Helper()
	f := notificationFixture{
		teacherID:   uuid.New(),
		contactID:   uuid.New(),
		periodID:    uuid.New(),
		statementID: uuid.New(),
	}
	require.NoError(t, db.Exec(
		`INSERT INTO user_accounts (id, role, phone) VALUES (?, 'teachers', ?)`,
		f.teacherID, phone).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO teachers (id, full_name) VALUES (?, 'Cô Lan')`,
		f.teacherID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, full_name, phone) VALUES (?, ?, 'Chị Hoa', ?)`,
		f.contactID, f.teacherID, phone).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO billing_periods (id, teacher_id, year, month, period_start, period_end)
		 VALUES (?, ?, 2026, 8, '2026-08-01', '2026-08-31')`,
		f.periodID, f.teacherID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, contact_id, period_id, token_hash, expires_at, total_due)
		 VALUES (?, ?, ?, ?, ?, now() + interval '7 days', 100000)`,
		f.statementID, f.teacherID, f.contactID, f.periodID, []byte("hash-"+phone)).Error)
	return f
}

// A notification may only point at a run of its own teacher, and losing the
// run must not delete the notification — it is the audit record of a message
// that reached a parent.
func TestNotificationRunLinkIsTenantScoped(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	teacherA := seedNotificationParents(t, db, "+84900000001")
	teacherB := seedNotificationParents(t, db, "+84900000002")

	runB := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO notification_runs (id, teacher_id, billing_period_id) VALUES (?, ?, ?)`,
		runB, teacherB.teacherID, teacherB.periodID).Error)

	// Teacher A's notification must not attach to teacher B's run.
	err = db.Exec(
		`INSERT INTO notifications (id, teacher_id, statement_id, channel, run_id)
		 VALUES (?, ?, ?, 'zalo_personal', ?)`,
		uuid.New(), teacherA.teacherID, teacherA.statementID, runB).Error
	require.Error(t, err, "cross-tenant run_id must be rejected by the database")

	// The same shape within one tenant is fine.
	runA := uuid.New()
	notifA := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO notification_runs (id, teacher_id, billing_period_id) VALUES (?, ?, ?)`,
		runA, teacherA.teacherID, teacherA.periodID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO notifications (id, teacher_id, statement_id, channel, run_id)
		 VALUES (?, ?, ?, 'zalo_personal', ?)`,
		notifA, teacherA.teacherID, teacherA.statementID, runA).Error)

	// Deleting the run detaches the notification instead of taking it along.
	require.NoError(t, db.Exec(`DELETE FROM notification_runs WHERE id = ?`, runA).Error)
	var runID *string
	require.NoError(t, db.Raw(
		`SELECT run_id FROM notifications WHERE id = ?`, notifA).Scan(&runID).Error)
	require.Nil(t, runID, "run_id must be nulled, not cascade-delete the notification")
	var channel string
	require.NoError(t, db.Raw(
		`SELECT channel FROM notifications WHERE id = ?`, notifA).Scan(&channel).Error)
	require.Equal(t, "zalo_personal", channel, "the notification row itself must survive")
}

// Rolling the schema back must fold zalo_personal rows into zalo_manual so the
// restored CHECK constraint validates — proven with a real row, not just DDL.
func TestDownFoldsPersonalChannelIntoManual(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	f := seedNotificationParents(t, db, "+84900000003")
	notifID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO notifications (id, teacher_id, statement_id, channel)
		 VALUES (?, ?, ?, 'zalo_personal')`,
		notifID, f.teacherID, f.statementID).Error)

	require.NoError(t, database.MigrateDown(m, 4))

	var channel string
	require.NoError(t, db.Raw(
		`SELECT channel FROM notifications WHERE id = ?`, notifID).Scan(&channel).Error)
	require.Equal(t, "zalo_manual", channel)

	var channelCheck string
	require.NoError(t, db.Raw(
		`SELECT pg_get_constraintdef(oid) FROM pg_constraint
		 WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_channel_check'`,
	).Scan(&channelCheck).Error)
	require.NotContains(t, channelCheck, "zalo_personal",
		"the restored CHECK must be the pre-migration one")
}

// The database itself enforces one live sending pass per teacher: the
// in-process guard cannot see a second API instance, so a partial unique
// index is the cross-process backstop.
func TestOneRunningRunPerTeacher(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	teacherA := seedNotificationParents(t, db, "+84900000004")
	teacherB := seedNotificationParents(t, db, "+84900000005")

	insertRun := func(f notificationFixture, status string) error {
		return db.Exec(
			`INSERT INTO notification_runs (id, teacher_id, billing_period_id, status)
			 VALUES (?, ?, ?, ?)`,
			uuid.New(), f.teacherID, f.periodID, status).Error
	}

	require.NoError(t, insertRun(teacherA, "running"))
	require.Error(t, insertRun(teacherA, "running"),
		"a second running run for the same teacher must be rejected")
	require.NoError(t, insertRun(teacherB, "running"),
		"another teacher's running run is unrelated")
	require.NoError(t, insertRun(teacherA, "completed"),
		"finished runs do not occupy the slot — only 'running' is unique")
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
