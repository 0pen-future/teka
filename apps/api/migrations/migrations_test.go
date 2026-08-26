//go:build integration

package migrations_test

import (
	"context"
	"fmt"
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
// refresh_tokens table from 000002, zalo_accounts from 000004, and
// audit_logs from 000010.
// schema_migrations belongs to golang-migrate itself.
var domainTables = []string{
	"user_accounts", "teachers", "contacts", "students", "classes",
	"class_schedules", "enrollments", "class_sessions", "attendance_records",
	"billing_periods", "invoices", "invoice_lines", "invoice_adjustments",
	"payments", "payment_allocations", "statements", "notifications",
	"refresh_tokens", "zalo_accounts", "notification_runs", "centers",
	"center_members", "invitations", "password_reset_tokens",
	"class_curricula", "lesson_plans", "session_notes", "session_marks",
	"audit_logs",
}

// centerTables is every business table 000007 re-keyed to the center tenant.
// Each must carry center_id, and every row's center_id must agree with the
// center of the teacher attributed on the same row.
var centerTables = []string{
	"contacts", "students", "classes", "class_schedules", "enrollments",
	"class_sessions", "attendance_records", "billing_periods", "invoices",
	"invoice_lines", "invoice_adjustments", "payments", "payment_allocations",
	"statements", "notifications", "notification_runs",
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
// teacher (inside their personal center), contact, billing period, statement.
type notificationFixture struct {
	teacherID   uuid.UUID
	centerID    uuid.UUID
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
		centerID:    uuid.New(),
		contactID:   uuid.New(),
		periodID:    uuid.New(),
		statementID: uuid.New(),
	}
	require.NoError(t, db.Exec(
		`INSERT INTO user_accounts (id, role, phone) VALUES (?, 'teachers', ?)`,
		f.teacherID, phone).Error)
	// A center, its first owner, and the owner's membership row are born
	// together: centers.owner_id and teachers' membership FK are both
	// DEFERRABLE so the trio can be inserted in one transaction — the same
	// shape the registration flow uses.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO centers (id, name, owner_id) VALUES (?, 'Cô Lan', ?)`,
			f.centerID, f.teacherID).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`INSERT INTO teachers (id, full_name, center_id) VALUES (?, 'Cô Lan', ?)`,
			f.teacherID, f.centerID).Error; err != nil {
			return err
		}
		return tx.Exec(
			`INSERT INTO center_members (teacher_id, center_id) VALUES (?, ?)`,
			f.teacherID, f.centerID).Error
	}))
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone) VALUES (?, ?, ?, 'Chị Hoa', ?)`,
		f.contactID, f.teacherID, f.centerID, phone).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO billing_periods (id, teacher_id, center_id, year, month, period_start, period_end)
		 VALUES (?, ?, ?, 2026, 8, '2026-08-01', '2026-08-31')`,
		f.periodID, f.teacherID, f.centerID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, token_hash, expires_at, total_due)
		 VALUES (?, ?, ?, ?, ?, ?, now() + interval '7 days', 100000)`,
		f.statementID, f.teacherID, f.centerID, f.contactID, f.periodID, []byte("hash-"+phone)).Error)
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
		`INSERT INTO notification_runs (id, teacher_id, center_id, billing_period_id) VALUES (?, ?, ?, ?)`,
		runB, teacherB.teacherID, teacherB.centerID, teacherB.periodID).Error)

	// Teacher A's notification must not attach to teacher B's run — the two
	// fixtures live in different centers, so the (run_id, center_id) FK trips.
	err = db.Exec(
		`INSERT INTO notifications (id, teacher_id, center_id, statement_id, channel, run_id)
		 VALUES (?, ?, ?, ?, 'zalo_personal', ?)`,
		uuid.New(), teacherA.teacherID, teacherA.centerID, teacherA.statementID, runB).Error
	require.Error(t, err, "cross-tenant run_id must be rejected by the database")

	// The same shape within one tenant is fine.
	runA := uuid.New()
	notifA := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO notification_runs (id, teacher_id, center_id, billing_period_id) VALUES (?, ?, ?, ?)`,
		runA, teacherA.teacherID, teacherA.centerID, teacherA.periodID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO notifications (id, teacher_id, center_id, statement_id, channel, run_id)
		 VALUES (?, ?, ?, ?, 'zalo_personal', ?)`,
		notifA, teacherA.teacherID, teacherA.centerID, teacherA.statementID, runA).Error)

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
		`INSERT INTO notifications (id, teacher_id, center_id, statement_id, channel)
		 VALUES (?, ?, ?, ?, 'zalo_personal')`,
		notifID, f.teacherID, f.centerID, f.statementID).Error)

	// Roll back through 000005 (zalo_personal_mapping): six steps now that the
	// additive 000008 sits on top of the seven migrations this test predates.
	require.NoError(t, database.MigrateDown(m, 6))

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
			`INSERT INTO notification_runs (id, teacher_id, center_id, billing_period_id, status)
			 VALUES (?, ?, ?, ?, ?)`,
			uuid.New(), f.teacherID, f.centerID, f.periodID, status).Error
	}

	require.NoError(t, insertRun(teacherA, "running"))
	require.Error(t, insertRun(teacherA, "running"),
		"a second running run for the same teacher must be rejected")
	require.NoError(t, insertRun(teacherB, "running"),
		"another teacher's running run is unrelated")
	require.NoError(t, insertRun(teacherA, "completed"),
		"finished runs do not occupy the slot — only 'running' is unique")
}

// seedLegacyTenant inserts a teacher in the pre-000007 shape — no centers, no
// center_id anywhere — so the 000007 backfill runs against real data instead
// of an empty database. full=true covers every business table with at least
// one row; full=false seeds only a thin slice to prove rows land in their own
// teacher's center, not someone else's.
func seedLegacyTenant(t *testing.T, db *gorm.DB, phone string, full bool) uuid.UUID {
	t.Helper()
	teacherID := uuid.New()
	contactID := uuid.New()
	classID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO user_accounts (id, role, phone) VALUES (?, 'teachers', ?)`,
		teacherID, phone).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO teachers (id, full_name) VALUES (?, 'Cô Mai')`,
		teacherID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, full_name, phone) VALUES (?, ?, 'Chị Hoa', ?)`,
		contactID, teacherID, phone).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO classes (id, teacher_id, name, start_date, default_unit_price)
		 VALUES (?, ?, 'Lớp Toán 9', '2026-01-05', 100000)`,
		classID, teacherID).Error)
	if !full {
		return teacherID
	}
	studentID := uuid.New()
	enrollmentID := uuid.New()
	sessionID := uuid.New()
	periodID := uuid.New()
	invoiceID := uuid.New()
	paymentID := uuid.New()
	statementID := uuid.New()
	runID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO students (id, teacher_id, contact_id, full_name) VALUES (?, ?, ?, 'Bé An')`,
		studentID, teacherID, contactID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO class_schedules (id, teacher_id, class_id, weekday, start_time, effective_from)
		 VALUES (?, ?, ?, 1, '18:00', '2026-01-05')`,
		uuid.New(), teacherID, classID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO enrollments (id, teacher_id, student_id, class_id, started_on, unit_price)
		 VALUES (?, ?, ?, ?, '2026-01-05', 100000)`,
		enrollmentID, teacherID, studentID, classID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO class_sessions (id, teacher_id, class_id, session_date, status, attendance_confirmed_at)
		 VALUES (?, ?, ?, '2026-08-03', 'held', now())`,
		sessionID, teacherID, classID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO attendance_records (id, teacher_id, session_id, student_id, enrollment_id, status)
		 VALUES (?, ?, ?, ?, ?, 'present')`,
		uuid.New(), teacherID, sessionID, studentID, enrollmentID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO billing_periods (id, teacher_id, year, month, period_start, period_end)
		 VALUES (?, ?, 2026, 8, '2026-08-01', '2026-08-31')`,
		periodID, teacherID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO invoices (id, teacher_id, period_id, student_id, contact_id, student_name, contact_name,
		                       opening_balance, current_charge, adjustment_total, total_due, status)
		 VALUES (?, ?, ?, ?, ?, 'Bé An', 'Chị Hoa', 0, 200000, -50000, 150000, 'issued')`,
		invoiceID, teacherID, periodID, studentID, contactID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO invoice_lines (id, teacher_id, invoice_id, enrollment_id, class_name, billable_count, unit_price, amount)
		 VALUES (?, ?, ?, ?, 'Lớp Toán 9', 2, 100000, 200000)`,
		uuid.New(), teacherID, invoiceID, enrollmentID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO invoice_adjustments (id, teacher_id, invoice_id, amount, reason)
		 VALUES (?, ?, ?, -50000, 'Giảm trừ buổi nghỉ có phép')`,
		uuid.New(), teacherID, invoiceID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO payments (id, teacher_id, contact_id, amount, received_on)
		 VALUES (?, ?, ?, 100000, '2026-08-05')`,
		paymentID, teacherID, contactID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO payment_allocations (id, teacher_id, payment_id, invoice_id, amount)
		 VALUES (?, ?, ?, ?, 100000)`,
		uuid.New(), teacherID, paymentID, invoiceID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, contact_id, period_id, token_hash, expires_at, total_due)
		 VALUES (?, ?, ?, ?, ?, now() + interval '7 days', 150000)`,
		statementID, teacherID, contactID, periodID, []byte("legacy-hash-"+phone)).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO notification_runs (id, teacher_id, billing_period_id) VALUES (?, ?, ?)`,
		runID, teacherID, periodID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO notifications (id, teacher_id, statement_id, channel, run_id)
		 VALUES (?, ?, ?, 'zalo_personal', ?)`,
		uuid.New(), teacherID, statementID, runID).Error)
	return teacherID
}

// The 000007 backfill must give every pre-existing teacher a personal center
// and stamp that center onto every business row the teacher owns — proven
// against data seeded in the old shape, not an empty database.
func TestCenterTenancyBackfill(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))
	// Step back below 000007 (the center tenancy migration), seed the old
	// teacher-tenant shape, then migrate up again so the backfill runs over
	// that data. Targeting version 6 directly keeps this stable as later
	// migrations land on top.
	require.NoError(t, m.Migrate(6))

	db := openDB(t, url)
	teacherFull := seedLegacyTenant(t, db, "+84900000101", true)
	teacherThin := seedLegacyTenant(t, db, "+84900000102", false)
	require.NoError(t, database.MigrateUp(m))

	// One personal center per pre-existing teacher, owned by that teacher.
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM centers`).Scan(&n).Error)
	require.EqualValues(t, 2, n, "backfill must create exactly one center per teacher")
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM teachers tt JOIN centers c ON c.id = tt.center_id
		 WHERE c.owner_id <> tt.id`).Scan(&n).Error)
	require.Zero(t, n, "every teacher must own their personal center")
	require.NoError(t, db.Raw(
		`SELECT count(DISTINCT center_id) FROM teachers WHERE id IN (?, ?)`,
		teacherFull, teacherThin).Scan(&n).Error)
	require.EqualValues(t, 2, n, "the two teachers must land in two different centers")

	// Membership history starts with exactly one live row per teacher,
	// agreeing with the teacher's current center.
	require.NoError(t, db.Raw(`SELECT count(*) FROM center_members`).Scan(&n).Error)
	require.EqualValues(t, 2, n, "backfill must create exactly one membership row per teacher")
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM teachers tt
		 WHERE NOT EXISTS (
		     SELECT 1 FROM center_members cm
		     WHERE cm.teacher_id = tt.id AND cm.center_id = tt.center_id
		       AND cm.left_at IS NULL)`).Scan(&n).Error)
	require.Zero(t, n, "every teacher must hold a live membership in their current center")

	// Every business row's center must agree with the center of its teacher —
	// checked per table, and each table must actually hold seeded rows so the
	// zero-mismatch assertion means something.
	for _, tbl := range centerTables {
		require.NoError(t, db.Raw(fmt.Sprintf(
			`SELECT count(*) FROM %s x JOIN teachers tt ON tt.id = x.teacher_id
			 WHERE x.center_id IS DISTINCT FROM tt.center_id`, tbl)).Scan(&n).Error)
		require.Zerof(t, n, "table %s has rows whose center_id disagrees with their teacher's center", tbl)
		require.NoError(t, db.Raw(fmt.Sprintf(`SELECT count(*) FROM %s`, tbl)).Scan(&n).Error)
		require.Positivef(t, n, "table %s has no seeded rows — the backfill check proved nothing", tbl)
	}

	// The rebuilt views carry the new tenant key.
	for _, v := range views {
		cols := nameSet(t, db, fmt.Sprintf(
			`SELECT column_name FROM information_schema.columns WHERE table_name = '%s'`, v))
		require.Truef(t, cols["center_id"], "view %s must expose center_id", v)
	}
}

// The composite FK (teacher_id, center_id) → center_members is the database's
// own guard that a row can never attribute a teacher who was never a member of
// its center, and uq_centers_owner keeps one live center per owner.
func TestCenterGuardsRejectCrossCenterRows(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	a := seedNotificationParents(t, db, "+84900000201")
	b := seedNotificationParents(t, db, "+84900000202")

	err = db.Exec(
		`INSERT INTO classes (id, teacher_id, center_id, name, start_date, default_unit_price)
		 VALUES (?, ?, ?, 'Lớp lệch center', '2026-01-05', 100000)`,
		uuid.New(), a.teacherID, b.centerID).Error
	require.ErrorContains(t, err, "fk_classes_teacher_center",
		"a row pairing a teacher with another center must be rejected by the guard FK")

	err = db.Exec(
		`INSERT INTO centers (id, name, owner_id) VALUES (?, 'Trung tâm thứ hai', ?)`,
		uuid.New(), a.teacherID).Error
	require.ErrorContains(t, err, "uq_centers_owner",
		"one owner cannot hold two live centers")
}

// Leaving a center is an UPDATE of left_at, never a DELETE: the guard FK is
// anchored on membership history, so a teacher can move on while every row
// they created stays behind in the old center, still attributed to them.
func TestTeacherLeavesCenterDataStaysBehind(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	a := seedNotificationParents(t, db, "+84900000301")
	b := seedNotificationParents(t, db, "+84900000302")

	// b joins a's center: close the personal membership first (only one may be
	// live at a time), open the new one, repoint the current-center column.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`UPDATE center_members SET left_at = now() WHERE teacher_id = ? AND center_id = ?`,
			b.teacherID, b.centerID).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`INSERT INTO center_members (teacher_id, center_id) VALUES (?, ?)`,
			b.teacherID, a.centerID).Error; err != nil {
			return err
		}
		return tx.Exec(
			`UPDATE teachers SET center_id = ? WHERE id = ?`, a.centerID, b.teacherID).Error
	}))

	// b teaches a class inside a's center.
	classID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO classes (id, teacher_id, center_id, name, start_date, default_unit_price)
		 VALUES (?, ?, ?, 'Lớp Văn 8', '2026-01-05', 100000)`,
		classID, b.teacherID, a.centerID).Error)

	// b leaves, back to the personal center. The class row keeps referencing
	// the now-closed membership, so this must succeed with the data in place.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`UPDATE center_members SET left_at = now() WHERE teacher_id = ? AND center_id = ?`,
			b.teacherID, a.centerID).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`UPDATE center_members SET left_at = NULL WHERE teacher_id = ? AND center_id = ?`,
			b.teacherID, b.centerID).Error; err != nil {
			return err
		}
		return tx.Exec(
			`UPDATE teachers SET center_id = ? WHERE id = ?`, b.centerID, b.teacherID).Error
	}))

	// The class stayed in a's center, still attributed to b.
	var got struct {
		TeacherID string
		CenterID  string
	}
	require.NoError(t, db.Raw(
		`SELECT teacher_id, center_id FROM classes WHERE id = ?`, classID).Scan(&got).Error)
	require.Equal(t, b.teacherID.String(), got.TeacherID,
		"the row must keep crediting the teacher who left")
	require.Equal(t, a.centerID.String(), got.CenterID,
		"the row must stay in the center it was created in")
}

// Hard-deleting an account must stay possible in one transaction: the
// deferred centers.owner_id check tolerates the teacher disappearing as long
// as the personal center goes in the same commit, and the membership rows
// cascade every business row along the way.
func TestTeacherHardDeleteInOneTransaction(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	f := seedNotificationParents(t, db, "+84900000401")

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM user_accounts WHERE id = ?`, f.teacherID).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM centers WHERE id = ?`, f.centerID).Error
	}))

	var n int64
	for _, tbl := range []string{
		"user_accounts", "teachers", "centers", "center_members",
		"contacts", "billing_periods", "statements",
	} {
		require.NoError(t, db.Raw(fmt.Sprintf(`SELECT count(*) FROM %s`, tbl)).Scan(&n).Error)
		require.Zerof(t, n, "table %s must be empty after the hard delete", tbl)
	}
}

// The 000008 partial unique indexes enforce the invite/reset invariants at the
// database, not just in service logic: at most one live (pending) invitation
// per (center, phone), and at most one live reset token per account.
func TestInviteOnlyOnboardingSchemaInvariants(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	a := seedNotificationParents(t, db, "+84900000501")
	b := seedNotificationParents(t, db, "+84900000502")

	insertInvite := func(centerID uuid.UUID, phone, hash, status string) error {
		return db.Exec(
			`INSERT INTO invitations (center_id, phone, token_hash, status, expires_at)
			 VALUES (?, ?, ?, ?, now() + interval '72 hours')`,
			centerID, phone, hash, status).Error
	}

	// One pending invite per (center, phone); a second pending for the same pair
	// trips uq_invitations_pending_phone.
	require.NoError(t, insertInvite(a.centerID, "+84911111111", "invhash-1", "pending"))
	require.ErrorContains(t,
		insertInvite(a.centerID, "+84911111111", "invhash-2", "pending"),
		"uq_invitations_pending_phone",
		"a second pending invite for the same (center, phone) must be rejected")
	// The same phone in a *different* center is unrelated.
	require.NoError(t, insertInvite(b.centerID, "+84911111111", "invhash-3", "pending"),
		"a pending invite for the same phone in another center is allowed")
	// A revoked invite frees the slot for a fresh pending one (re-invite).
	require.NoError(t, db.Exec(
		`UPDATE invitations SET status = 'revoked', revoked_at = now()
		 WHERE center_id = ? AND phone = '+84911111111'`, a.centerID).Error)
	require.NoError(t, insertInvite(a.centerID, "+84911111111", "invhash-4", "pending"),
		"once the prior invite is revoked, a new pending invite is allowed")

	insertReset := func(userID uuid.UUID, hash string) error {
		return db.Exec(
			`INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
			 VALUES (?, ?, now() + interval '48 hours')`,
			userID, hash).Error
	}

	// One live reset token per account; a second live one trips
	// uq_password_reset_active.
	require.NoError(t, insertReset(a.teacherID, "resethash-1"))
	require.ErrorContains(t, insertReset(a.teacherID, "resethash-2"),
		"uq_password_reset_active",
		"a second live reset token for one account must be rejected")
	// Superseding the first frees the slot; so does consuming it.
	require.NoError(t, db.Exec(
		`UPDATE password_reset_tokens SET superseded_at = now()
		 WHERE user_id = ? AND token_hash = 'resethash-1'`, a.teacherID).Error)
	require.NoError(t, insertReset(a.teacherID, "resethash-3"),
		"once the prior token is superseded, a new live token is allowed")
	require.NoError(t, db.Exec(
		`UPDATE password_reset_tokens SET used_at = now()
		 WHERE user_id = ? AND token_hash = 'resethash-3'`, a.teacherID).Error)
	require.NoError(t, insertReset(a.teacherID, "resethash-4"),
		"once the prior token is consumed, a new live token is allowed")
}

// teachingFixture extends the notification parent chain with the class /
// session / student rows the 000009 teaching tables hang off of.
type teachingFixture struct {
	notificationFixture
	classID   uuid.UUID
	sessionID uuid.UUID
	studentID uuid.UUID
}

func seedTeachingParents(t *testing.T, db *gorm.DB, phone string) teachingFixture {
	t.Helper()
	f := teachingFixture{
		notificationFixture: seedNotificationParents(t, db, phone),
		classID:             uuid.New(),
		sessionID:           uuid.New(),
		studentID:           uuid.New(),
	}
	require.NoError(t, db.Exec(
		`INSERT INTO classes (id, teacher_id, center_id, name, start_date, default_unit_price)
		 VALUES (?, ?, ?, 'Lớp Toán 9', '2026-01-05', 100000)`,
		f.classID, f.teacherID, f.centerID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO class_sessions (id, teacher_id, center_id, class_id, session_date)
		 VALUES (?, ?, ?, ?, '2026-08-10')`,
		f.sessionID, f.teacherID, f.centerID, f.classID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO students (id, teacher_id, center_id, contact_id, full_name)
		 VALUES (?, ?, ?, ?, 'Bé An')`,
		f.studentID, f.teacherID, f.centerID, f.contactID).Error)
	return f
}

// The 000009 teaching tables carry the same center integrity as every other
// business table: the membership guard rejects cross-center rows, the CHECKs
// hold the UI's value ranges, and rows follow their parents on hard delete.
func TestTeachingTablesIntegrity(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	a := seedTeachingParents(t, db, "+84900000601")
	b := seedTeachingParents(t, db, "+84900000602")

	// Membership guard: a row pairing a's teacher with b's center must fail.
	err = db.Exec(
		`INSERT INTO class_curricula (id, class_id, teacher_id, center_id)
		 VALUES (?, ?, ?, ?)`,
		uuid.New(), b.classID, a.teacherID, b.centerID).Error
	require.ErrorContains(t, err, "teacher_center",
		"a teaching row pairing a teacher with another center must be rejected")

	// One curriculum per class.
	require.NoError(t, db.Exec(
		`INSERT INTO class_curricula (id, class_id, teacher_id, center_id, lessons, current_index)
		 VALUES (?, ?, ?, ?, '["Bài 1", "Bài 2"]', 1)`,
		uuid.New(), a.classID, a.teacherID, a.centerID).Error)
	require.Error(t, db.Exec(
		`INSERT INTO class_curricula (id, class_id, teacher_id, center_id)
		 VALUES (?, ?, ?, ?)`,
		uuid.New(), a.classID, a.teacherID, a.centerID).Error,
		"a second curriculum for the same class must be rejected")

	// Lesson plans: status is the four persisted states only ('none' is the
	// absence of a row), one plan per (class, lesson_index).
	planID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO lesson_plans (id, class_id, lesson_index, teacher_id, center_id, status, submitted_by, submitted_at)
		 VALUES (?, ?, 0, ?, ?, 'pending', ?, now())`,
		planID, a.classID, a.teacherID, a.centerID, a.teacherID).Error)
	require.Error(t, db.Exec(
		`INSERT INTO lesson_plans (id, class_id, lesson_index, teacher_id, center_id, status)
		 VALUES (?, ?, 0, ?, ?, 'draft')`,
		uuid.New(), a.classID, a.teacherID, a.centerID).Error,
		"a second plan for the same (class, lesson_index) must be rejected")
	require.Error(t, db.Exec(
		`INSERT INTO lesson_plans (id, class_id, lesson_index, teacher_id, center_id, status)
		 VALUES (?, ?, 1, ?, ?, 'none')`,
		uuid.New(), a.classID, a.teacherID, a.centerID).Error,
		"'none' is not a persisted status")
	require.Error(t, db.Exec(
		`INSERT INTO lesson_plans (id, class_id, lesson_index, teacher_id, center_id, status)
		 VALUES (?, ?, -1, ?, ?, 'draft')`,
		uuid.New(), a.classID, a.teacherID, a.centerID).Error,
		"negative lesson_index must be rejected")

	// Session note is 1:1 with its session (PK), marks unique per student and
	// score capped at the UI's 0–10 scale.
	require.NoError(t, db.Exec(
		`INSERT INTO session_notes (session_id, teacher_id, center_id, body)
		 VALUES (?, ?, ?, 'Cả lớp học tốt')`,
		a.sessionID, a.teacherID, a.centerID).Error)
	require.Error(t, db.Exec(
		`INSERT INTO session_notes (session_id, teacher_id, center_id, body)
		 VALUES (?, ?, ?, 'trùng buổi')`,
		a.sessionID, a.teacherID, a.centerID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO session_marks (id, session_id, student_id, teacher_id, center_id, score, personal_note)
		 VALUES (?, ?, ?, ?, ?, 8.5, 'Tiến bộ')`,
		uuid.New(), a.sessionID, a.studentID, a.teacherID, a.centerID).Error)
	require.Error(t, db.Exec(
		`INSERT INTO session_marks (id, session_id, student_id, teacher_id, center_id, score)
		 VALUES (?, ?, ?, ?, ?, 7)`,
		uuid.New(), a.sessionID, a.studentID, a.teacherID, a.centerID).Error,
		"a second mark row for the same (session, student) must be rejected")
	require.Error(t, db.Exec(
		`INSERT INTO session_marks (id, session_id, student_id, teacher_id, center_id, score)
		 VALUES (?, ?, ?, ?, ?, 10.5)`,
		uuid.New(), b.sessionID, b.studentID, b.teacherID, b.centerID).Error,
		"score above 10 must be rejected")

	// Hard-deleting the class takes its curriculum, plans, sessions and their
	// notes/marks along — teaching data has no life of its own.
	require.NoError(t, db.Exec(`DELETE FROM classes WHERE id = ?`, a.classID).Error)
	var n int64
	for _, tbl := range []string{"class_curricula", "lesson_plans", "session_notes", "session_marks"} {
		require.NoError(t, db.Raw(fmt.Sprintf(
			`SELECT count(*) FROM %s WHERE center_id = ?`, tbl), a.centerID).Scan(&n).Error)
		require.Zerof(t, n, "table %s must be empty after the class hard delete", tbl)
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
