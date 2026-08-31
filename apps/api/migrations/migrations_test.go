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
	"teka/apps/api/internal/shared/authctx"
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
	"score_sets", "score_set_components", "class_score_components",
	"student_scores",
	"owner_anchor_backfill",
	"rbac_backfill_rows", "rbac_backfill_ledger",
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

	// Roll back through 000005 (zalo_personal_mapping): sixteen steps now
	// that the additive 000008-000020 sit on top of the migrations this test
	// predates.
	require.NoError(t, database.MigrateDown(m, 16))

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

// The 000015 backfill gives every live class an active giao_vien stint for its
// current teacher, skips soft-deleted classes, and is idempotent — re-running
// the INSERT must not duplicate stints.
func TestClassStaffBackfill(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	// Stop just below 000015, seed classes in the pre-class_staff shape, then
	// migrate up so the backfill runs over that data.
	require.NoError(t, m.Migrate(14))

	db := openDB(t, url)
	f := seedNotificationParents(t, db, "+84900000601")
	liveClass := uuid.New()
	deletedClass := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO classes (id, teacher_id, center_id, name, start_date, default_unit_price)
		 VALUES (?, ?, ?, 'Lớp sống', '2026-01-05', 100000)`,
		liveClass, f.teacherID, f.centerID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO classes (id, teacher_id, center_id, name, start_date, default_unit_price, deleted_at)
		 VALUES (?, ?, ?, 'Lớp đã xoá', '2026-01-05', 100000, now())`,
		deletedClass, f.teacherID, f.centerID).Error)

	require.NoError(t, database.MigrateUp(m))

	var rows []struct {
		ClassID   uuid.UUID
		TeacherID uuid.UUID
		RoleKey   string
	}
	require.NoError(t, db.Raw(
		`SELECT class_id, teacher_id, role_key FROM class_staff WHERE ended_at IS NULL`).Scan(&rows).Error)
	require.Len(t, rows, 1, "only the live class is backfilled")
	require.Equal(t, liveClass, rows[0].ClassID)
	require.Equal(t, f.teacherID, rows[0].TeacherID)
	require.Equal(t, "giao_vien", rows[0].RoleKey)

	// Idempotent: the backfill INSERT conflicts on the active-stint index and
	// changes nothing on a second run.
	require.NoError(t, db.Exec(`
		INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
		SELECT c.id, c.center_id, c.teacher_id, 'giao_vien'
		FROM classes c
		WHERE c.deleted_at IS NULL
		ON CONFLICT (class_id, teacher_id) WHERE ended_at IS NULL DO NOTHING`).Error)
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM class_staff`).Scan(&n).Error)
	require.EqualValues(t, 1, n)
}

// The two partial unique indexes are the only cross-process protection for the
// dual-write invariant: one active stint per person per class, and exactly one
// active giao_vien per class. Two concurrent handoffs must collide here instead
// of silently producing two primary teachers.
func TestClassStaffIndexesEnforceInvariants(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	f := seedNotificationParents(t, db, "+84900000602")
	memberID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO user_accounts (id, role, phone) VALUES (?, 'teachers', '+84900000603')`,
		memberID).Error)
	// teachers ⇄ center_members reference each other; the membership FK is
	// DEFERRABLE so the pair goes in as one transaction, like registration.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO teachers (id, full_name, center_id) VALUES (?, 'Thầy Minh', ?)`,
			memberID, f.centerID).Error; err != nil {
			return err
		}
		return tx.Exec(
			`INSERT INTO center_members (teacher_id, center_id) VALUES (?, ?)`,
			memberID, f.centerID).Error
	}))

	classID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO classes (id, teacher_id, center_id, name, start_date, default_unit_price)
		 VALUES (?, ?, ?, 'Lớp bất biến', '2026-01-05', 100000)`,
		classID, f.teacherID, f.centerID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
		 VALUES (?, ?, ?, 'giao_vien')`,
		classID, f.centerID, f.teacherID).Error)

	err = db.Exec(
		`INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
		 VALUES (?, ?, ?, 'giao_vien')`,
		classID, f.centerID, memberID).Error
	require.ErrorContains(t, err, "uq_class_staff_one_gv",
		"a second active giao_vien for the same class must be rejected")

	err = db.Exec(
		`INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
		 VALUES (?, ?, ?, 'hoc_vu')`,
		classID, f.centerID, f.teacherID).Error
	require.ErrorContains(t, err, "uq_class_staff_active",
		"one person cannot hold two active stints in the same class")

	// The predicates are scoped: another role for another member is fine, and
	// ending the giao_vien stint frees the class for a new primary teacher.
	require.NoError(t, db.Exec(
		`INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
		 VALUES (?, ?, ?, 'tro_giang')`,
		classID, f.centerID, memberID).Error)
	require.NoError(t, db.Exec(
		`UPDATE class_staff SET ended_at = now()
		 WHERE class_id = ? AND role_key IN ('giao_vien', 'tro_giang')`, classID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
		 VALUES (?, ?, ?, 'giao_vien')`,
		classID, f.centerID, memberID).Error)
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
	classID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO classes (id, teacher_id, center_id, name, start_date, default_unit_price)
		 VALUES (?, ?, ?, 'Lớp Toán 9', '2026-01-05', 100000)`,
		classID, f.teacherID, f.centerID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
		 VALUES (?, ?, ?, 'giao_vien')`,
		classID, f.centerID, f.teacherID).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM user_accounts WHERE id = ?`, f.teacherID).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM centers WHERE id = ?`, f.centerID).Error
	}))

	var n int64
	for _, tbl := range []string{
		"user_accounts", "teachers", "centers", "center_members",
		"contacts", "billing_periods", "statements", "class_staff",
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

// rbacMember seeds one member account plus a membership stint in the given
// center, in the pre-RBAC shape (no role_id column yet).
func rbacMember(t *testing.T, db *gorm.DB, centerID uuid.UUID, phone string, canSend, closed bool) uuid.UUID {
	t.Helper()
	teacherID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO user_accounts (id, role, phone) VALUES (?, 'teachers', ?)`,
		teacherID, phone).Error)
	leftAt := "NULL"
	if closed {
		leftAt = "now()"
	}
	// teachers and the membership stint reference each other through the
	// deferrable fk_teachers_membership, so both rows land in one transaction.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO teachers (id, full_name, center_id) VALUES (?, 'Giáo Viên', ?)`,
			teacherID, centerID).Error; err != nil {
			return err
		}
		return tx.Exec(fmt.Sprintf(
			`INSERT INTO center_members (teacher_id, center_id, can_send_reports, left_at)
			 VALUES (?, ?, ?, %s)`, leftAt),
			teacherID, centerID, canSend).Error
	}))
	return teacherID
}

// The RBAC backfill must be observed over pre-existing data: a plain
// integration test migrates an empty schema and can never see it. Step back to
// the pre-RBAC version, seed the old shape, migrate forward, and check what
// the backfill wrote.
func TestCenterRBACBackfill(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))
	// Step back below 000013 (the RBAC migration) and seed the pre-RBAC shape.
	require.NoError(t, m.Migrate(12))

	db := openDB(t, url)
	live := seedNotificationParents(t, db, "+84900000501")
	flagged := rbacMember(t, db, live.centerID, "+84900000502", true, false)
	plain := rbacMember(t, db, live.centerID, "+84900000503", false, false)
	// A closed stint that still carries the flag: dead state must not seed
	// grants or roles.
	former := rbacMember(t, db, live.centerID, "+84900000504", true, true)
	// A retired center gets no roles at all.
	retired := seedNotificationParents(t, db, "+84900000505")
	require.NoError(t, db.Exec(
		`UPDATE centers SET deleted_at = now() WHERE id = ?`, retired.centerID).Error)

	require.NoError(t, database.MigrateUp(m))

	// Every live center owns exactly its three system roles.
	roleNames := nameSet(t, db, fmt.Sprintf(
		`SELECT key FROM center_roles WHERE center_id = '%s'`, live.centerID))
	require.Equal(t, map[string]bool{"giao_vien": true, "hoc_vu": true, "tro_giang": true}, roleNames)
	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM center_roles WHERE center_id = ? AND NOT is_system`,
		live.centerID).Scan(&n).Error)
	require.Zero(t, n, "backfilled roles are all system roles")
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM center_roles WHERE center_id = ?`, retired.centerID).Scan(&n).Error)
	require.Zero(t, n, "a retired center gets no roles")

	// Live member stints land on giao_vien; the owner and closed stints stay
	// outside the role system.
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM center_members cm
		 JOIN center_roles cr ON cr.id = cm.role_id
		 WHERE cm.teacher_id IN (?, ?) AND cr.key = 'giao_vien' AND cr.center_id = ?`,
		flagged, plain, live.centerID).Scan(&n).Error)
	require.EqualValues(t, 2, n, "live member stints must get the default giao_vien role")
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM center_members
		 WHERE teacher_id IN (?, ?) AND role_id IS NOT NULL`,
		live.teacherID, former).Scan(&n).Error)
	require.Zero(t, n, "the owner and closed stints must keep a NULL role")

	// The can_send_reports flag becomes exactly one reports.send grant row —
	// live flagged stints only.
	var rows []struct {
		TeacherID uuid.UUID
		Allowed   bool
	}
	require.NoError(t, db.Raw(
		`SELECT teacher_id, allowed FROM center_member_permissions
		 WHERE permission_key = 'reports.send'`).Scan(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, flagged, rows[0].TeacherID)
	require.True(t, rows[0].Allowed)
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM center_member_permissions`).Scan(&n).Error)
	require.EqualValues(t, 1, n, "backfill writes nothing but the send-reports parity rows")
}

// Rolling the send-reports column back must rebuild it from the full
// effective verdict — (role grant ∪ member grant) − member deny — because
// role-level reports.send grants exist once the catalog stops blocking them;
// a member-rows-only rebuild would silently strip every role-granted sender.
func TestDownRestoresRoleGrantedSendReports(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	live := seedNotificationParents(t, db, "+84900000901")
	senderRole := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO center_roles (id, center_id, key, name) VALUES (?, ?, 'hoc_vu', 'Học vụ')`,
		senderRole, live.centerID).Error)
	member := func(phone string, roleID *uuid.UUID) uuid.UUID {
		teacherID := uuid.New()
		require.NoError(t, db.Exec(
			`INSERT INTO user_accounts (id, role, phone) VALUES (?, 'teachers', ?)`,
			teacherID, phone).Error)
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(
				`INSERT INTO teachers (id, full_name, center_id) VALUES (?, 'Giáo Viên', ?)`,
				teacherID, live.centerID).Error; err != nil {
				return err
			}
			return tx.Exec(
				`INSERT INTO center_members (teacher_id, center_id, role_id) VALUES (?, ?, ?)`,
				teacherID, live.centerID, roleID).Error
		}))
		return teacherID
	}
	roleGranted := member("+84900000902", &senderRole)
	memberGranted := member("+84900000903", nil)
	roleDenied := member("+84900000904", &senderRole)

	require.NoError(t, db.Exec(
		`INSERT INTO center_role_permissions (role_id, permission_key) VALUES (?, 'reports.send')`,
		senderRole).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		 VALUES (?, ?, 'reports.send', TRUE), (?, ?, 'reports.send', FALSE)`,
		memberGranted, live.centerID, roleDenied, live.centerID).Error)

	// Step below 000019: the column comes back rebuilt from the verdict.
	require.NoError(t, m.Migrate(18))

	canSend := func(teacherID uuid.UUID) bool {
		var v bool
		require.NoError(t, db.Raw(
			`SELECT can_send_reports FROM center_members WHERE teacher_id = ? AND center_id = ?`,
			teacherID, live.centerID).Scan(&v).Error)
		return v
	}
	require.True(t, canSend(roleGranted), "a role-granted sender must survive rollback")
	require.True(t, canSend(memberGranted), "a member-granted sender must survive rollback")
	require.False(t, canSend(roleDenied), "a member deny must beat the role grant")
}

// The 000016 anchor migration turns contacts into center-level data: duplicate
// live contacts per (center, phone) merge into one survivor, the two contact
// unique indexes re-key from per-teacher to per-center, and contacts plus
// students anchor to the center owner. The owner_anchor_backfill trail must be
// rich enough for down to restore anchors, revive merged losers with their
// zalo mapping intact, and put back de-duplicated zalo mappings.
func TestOwnerDataAnchorBackfill(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))
	// Step back below 000016 and seed the per-teacher shape it migrates.
	require.NoError(t, m.Migrate(15))

	db := openDB(t, url)
	owner := seedNotificationParents(t, db, "+84900000701")
	memberID := rbacMember(t, db, owner.centerID, "+84900000702", false, false)

	// Owner and member each saved the same parent — the supported duplicate
	// the merge step exists for. The member linked their copy on zalo.
	const sharedPhone = "+84903000701"
	ownerContact := uuid.New()
	memberContact := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone, created_at)
		 VALUES (?, ?, ?, 'Chị Hoa', ?, '2026-01-01')`,
		ownerContact, owner.teacherID, owner.centerID, sharedPhone).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone, created_at, zalo_user_id, zalo_name)
		 VALUES (?, ?, ?, 'Chị Hoa (GV)', ?, '2026-02-01', 'zalo-hoa', 'Hoa Zalo')`,
		memberContact, memberID, owner.centerID, sharedPhone).Error)

	// The member's family data hangs off the losing contact.
	memberStudent := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO students (id, teacher_id, center_id, contact_id, full_name)
		 VALUES (?, ?, ?, ?, 'Bé An')`,
		memberStudent, memberID, owner.centerID, memberContact).Error)
	memberPeriod := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO billing_periods (id, teacher_id, center_id, year, month, period_start, period_end)
		 VALUES (?, ?, ?, 2026, 8, '2026-08-01', '2026-08-31')`,
		memberPeriod, memberID, owner.centerID).Error)
	memberInvoice := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO invoices (id, teacher_id, center_id, period_id, student_id, contact_id, student_name, contact_name)
		 VALUES (?, ?, ?, ?, ?, ?, 'Bé An', 'Chị Hoa')`,
		memberInvoice, memberID, owner.centerID, memberPeriod, memberStudent, memberContact).Error)
	memberPayment := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO payments (id, teacher_id, center_id, contact_id, amount, received_on)
		 VALUES (?, ?, ?, ?, 50000, '2026-08-15')`,
		memberPayment, memberID, owner.centerID, memberContact).Error)

	// Statements for the losing contact: one in the owner's period where the
	// survivor already holds a live statement (collides after the merge — must
	// be soft-deleted), one in the member's own period (repoints cleanly,
	// keeping id and token so an issued parent link stays valid).
	survivorStmt := uuid.New()
	collidingStmt := uuid.New()
	soloStmt := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, token_hash, expires_at, total_due)
		 VALUES (?, ?, ?, ?, ?, ?, now() + interval '7 days', 100000)`,
		survivorStmt, owner.teacherID, owner.centerID, ownerContact, owner.periodID, []byte("hash-survivor")).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, token_hash, expires_at, total_due)
		 VALUES (?, ?, ?, ?, ?, ?, now() + interval '7 days', 100000)`,
		collidingStmt, memberID, owner.centerID, memberContact, owner.periodID, []byte("hash-collide")).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, token_hash, expires_at, total_due)
		 VALUES (?, ?, ?, ?, ?, ?, now() + interval '7 days', 100000)`,
		soloStmt, memberID, owner.centerID, memberContact, memberPeriod, []byte("hash-solo")).Error)

	// Two live contacts with different phones mapped to the same zalo friend —
	// legal per-teacher, illegal per-center. The earlier one keeps the mapping.
	contactZ1 := uuid.New()
	contactZ2 := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone, created_at, zalo_user_id, zalo_name)
		 VALUES (?, ?, ?, 'Anh Dũng', '+84903000702', '2026-01-05', 'zalo-dup-1', 'Dũng Zalo')`,
		contactZ1, owner.teacherID, owner.centerID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone, created_at, zalo_user_id, zalo_name)
		 VALUES (?, ?, ?, 'Anh Dũng (GV)', '+84903000703', '2026-03-01', 'zalo-dup-1', 'Dũng Zalo GV')`,
		contactZ2, memberID, owner.centerID).Error)

	require.NoError(t, database.MigrateUp(m))

	// Merge: the member's duplicate is soft-deleted, keeps its zalo mapping on
	// the dead row, and the anchor pass stamps the owner even there.
	var loser struct {
		TeacherID  uuid.UUID
		DeletedAt  *time.Time
		ZaloUserID *string
	}
	require.NoError(t, db.Raw(
		`SELECT teacher_id, deleted_at, zalo_user_id FROM contacts WHERE id = ?`,
		memberContact).Scan(&loser).Error)
	require.NotNil(t, loser.DeletedAt, "the losing duplicate must be soft-deleted")
	require.NotNil(t, loser.ZaloUserID, "the loser keeps its zalo mapping for revival")
	require.Equal(t, "zalo-hoa", *loser.ZaloUserID)
	require.Equal(t, owner.teacherID, loser.TeacherID)

	// Children repointed to the survivor.
	for _, q := range []struct {
		table string
		id    uuid.UUID
	}{
		{"students", memberStudent},
		{"invoices", memberInvoice},
		{"payments", memberPayment},
	} {
		var child struct{ ContactID uuid.UUID }
		require.NoError(t, db.Raw(fmt.Sprintf(
			`SELECT contact_id FROM %s WHERE id = ?`, q.table), q.id).Scan(&child).Error)
		require.Equalf(t, ownerContact, child.ContactID, "%s row must repoint to the surviving contact", q.table)
	}

	// Statements: the period collision is soft-deleted (the survivor's copy is
	// canonical), the solo one repoints and stays live.
	var stmt struct {
		ContactID uuid.UUID
		DeletedAt *time.Time
	}
	require.NoError(t, db.Raw(
		`SELECT contact_id, deleted_at FROM statements WHERE id = ?`, collidingStmt).Scan(&stmt).Error)
	require.Equal(t, ownerContact, stmt.ContactID)
	require.NotNil(t, stmt.DeletedAt, "the statement colliding with the survivor's period must be soft-deleted")
	require.NoError(t, db.Raw(
		`SELECT contact_id, deleted_at FROM statements WHERE id = ?`, soloStmt).Scan(&stmt).Error)
	require.Equal(t, ownerContact, stmt.ContactID)
	require.Nil(t, stmt.DeletedAt, "the non-colliding statement must stay live under the survivor")

	// Zalo dedupe: the earlier mapping survives, the later one is removed with
	// its old values recorded for down.
	var zalo *string
	require.NoError(t, db.Raw(
		`SELECT zalo_user_id FROM contacts WHERE id = ?`, contactZ1).Scan(&zalo).Error)
	require.NotNil(t, zalo)
	require.Equal(t, "zalo-dup-1", *zalo)
	require.NoError(t, db.Raw(
		`SELECT zalo_user_id FROM contacts WHERE id = ?`, contactZ2).Scan(&zalo).Error)
	require.Nil(t, zalo, "the later duplicate mapping must be cleared")
	var trail struct {
		OldZaloUserID string
		OldZaloName   string
	}
	require.NoError(t, db.Raw(
		`SELECT old_zalo_user_id, old_zalo_name FROM owner_anchor_backfill
		 WHERE table_name = 'contacts_zalo' AND row_id = ?`, contactZ2).Scan(&trail).Error)
	require.Equal(t, "zalo-dup-1", trail.OldZaloUserID)
	require.Equal(t, "Dũng Zalo GV", trail.OldZaloName)

	// Anchor: no contact or student row may disagree with the center owner.
	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM contacts c JOIN centers ce ON ce.id = c.center_id
		 WHERE c.teacher_id <> ce.owner_id`).Scan(&n).Error)
	require.Zero(t, n, "every contact must anchor to the center owner")
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM students s JOIN centers ce ON ce.id = s.center_id
		 WHERE s.teacher_id <> ce.owner_id`).Scan(&n).Error)
	require.Zero(t, n, "every student must anchor to the center owner")

	// The deploy-runbook collision counts must both be zero after up.
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM (
		     SELECT 1 FROM contacts WHERE deleted_at IS NULL
		     GROUP BY center_id, phone HAVING count(*) > 1) g`).Scan(&n).Error)
	require.Zero(t, n, "no live (center, phone) duplicates may remain")
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM (
		     SELECT 1 FROM contacts WHERE deleted_at IS NULL AND zalo_user_id IS NOT NULL
		     GROUP BY center_id, zalo_user_id HAVING count(*) > 1) g`).Scan(&n).Error)
	require.Zero(t, n, "no live (center, zalo_user_id) duplicates may remain")

	// Re-keyed uniqueness: the same phone or zalo friend under ANOTHER teacher
	// of the same center is now rejected — per-teacher it was allowed.
	err = db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone)
		 VALUES (?, ?, ?, 'Trùng phone', ?)`,
		uuid.New(), memberID, owner.centerID, sharedPhone).Error
	require.Error(t, err, "a live duplicate phone within the center must be rejected")
	err = db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone, zalo_user_id)
		 VALUES (?, ?, ?, 'Trùng zalo', '+84903000999', 'zalo-dup-1')`,
		uuid.New(), memberID, owner.centerID).Error
	require.Error(t, err, "a live duplicate zalo mapping within the center must be rejected")

	// Down: anchors restored, the merged loser revived with mapping intact,
	// the de-duplicated zalo mapping put back, children left with the survivor.
	require.NoError(t, m.Migrate(15))

	require.NoError(t, db.Raw(
		`SELECT teacher_id, deleted_at, zalo_user_id FROM contacts WHERE id = ?`,
		memberContact).Scan(&loser).Error)
	require.Nil(t, loser.DeletedAt, "down must revive the merged loser")
	require.Equal(t, memberID, loser.TeacherID, "down must restore the loser's original teacher")
	require.NotNil(t, loser.ZaloUserID)
	require.Equal(t, "zalo-hoa", *loser.ZaloUserID, "the revived loser keeps its zalo mapping")

	var student struct {
		TeacherID uuid.UUID
		ContactID uuid.UUID
	}
	require.NoError(t, db.Raw(
		`SELECT teacher_id, contact_id FROM students WHERE id = ?`, memberStudent).Scan(&student).Error)
	require.Equal(t, memberID, student.TeacherID, "down must restore the student's original teacher")

	require.NoError(t, db.Raw(
		`SELECT zalo_user_id FROM contacts WHERE id = ?`, contactZ2).Scan(&zalo).Error)
	require.NotNil(t, zalo, "down must restore the de-duplicated zalo mapping")
	require.Equal(t, "zalo-dup-1", *zalo)

	// Documented best-effort: children merged onto the survivor stay there,
	// and the collided statement stays soft-deleted.
	require.Equal(t, ownerContact, student.ContactID, "repointed children stay with the survivor after down")
	require.NoError(t, db.Raw(
		`SELECT contact_id, deleted_at FROM statements WHERE id = ?`, collidingStmt).Scan(&stmt).Error)
	require.NotNil(t, stmt.DeletedAt, "the collided statement stays soft-deleted after down")

	require.False(t, tableNames(t, db)["owner_anchor_backfill"],
		"down must drop the backfill table")
}

// Three live contacts share one phone and TWO losers each hold a live
// statement in the same period while the survivor holds none: comparing
// losers only against the survivor would soft-delete neither, and the repoint
// would then collide on uq_statements. The dedupe must work per merge group
// and period — keep the earliest statement, soft-delete the rest — so the
// migration completes with exactly one live statement per (contact, period).
func TestOwnerDataAnchorStatementDedupeAcrossLosers(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))
	require.NoError(t, m.Migrate(15))

	db := openDB(t, url)
	owner := seedNotificationParents(t, db, "+84900000801")
	member1 := rbacMember(t, db, owner.centerID, "+84900000802", false, false)
	member2 := rbacMember(t, db, owner.centerID, "+84900000803", false, false)

	// Same parent saved three times; the owner's copy is oldest → survivor.
	const sharedPhone = "+84903000801"
	survivorContact := uuid.New()
	loserContactB := uuid.New()
	loserContactC := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone, created_at)
		 VALUES (?, ?, ?, 'Chị Hạnh', ?, '2026-01-01')`,
		survivorContact, owner.teacherID, owner.centerID, sharedPhone).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone, created_at)
		 VALUES (?, ?, ?, 'Chị Hạnh (GV1)', ?, '2026-02-01')`,
		loserContactB, member1, owner.centerID, sharedPhone).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone, created_at)
		 VALUES (?, ?, ?, 'Chị Hạnh (GV2)', ?, '2026-03-01')`,
		loserContactC, member2, owner.centerID, sharedPhone).Error)

	// Both losers issued a statement for the same period; the survivor issued
	// none. C's statement is older and must be the surviving canonical copy.
	stmtB := uuid.New()
	stmtC := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, token_hash, expires_at, total_due, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, now() + interval '7 days', 100000, '2026-08-02')`,
		stmtB, member1, owner.centerID, loserContactB, owner.periodID, []byte("hash-loser-b")).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, token_hash, expires_at, total_due, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, now() + interval '7 days', 100000, '2026-08-01')`,
		stmtC, member2, owner.centerID, loserContactC, owner.periodID, []byte("hash-loser-c")).Error)

	// The migration itself completing is the core assertion: with a
	// survivor-only comparison both statements would repoint live and trip
	// uq_statements, leaving the migrator dirty.
	require.NoError(t, database.MigrateUp(m))

	var stmt struct {
		ContactID uuid.UUID
		DeletedAt *time.Time
	}
	require.NoError(t, db.Raw(
		`SELECT contact_id, deleted_at FROM statements WHERE id = ?`, stmtC).Scan(&stmt).Error)
	require.Equal(t, survivorContact, stmt.ContactID)
	require.Nil(t, stmt.DeletedAt, "the earliest statement of the merge group must stay live")
	require.NoError(t, db.Raw(
		`SELECT contact_id, deleted_at FROM statements WHERE id = ?`, stmtB).Scan(&stmt).Error)
	require.Equal(t, survivorContact, stmt.ContactID)
	require.NotNil(t, stmt.DeletedAt, "the later duplicate must be soft-deleted")

	// Exactly one live statement per (contact, period) across the database.
	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM (
		     SELECT 1 FROM statements WHERE deleted_at IS NULL
		     GROUP BY contact_id, period_id HAVING count(*) > 1) g`).Scan(&n).Error)
	require.Zero(t, n, "no live (contact, period) statement duplicates may remain")

	// Both losers merged into the survivor and every contact anchors to the
	// owner.
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM owner_anchor_backfill
		 WHERE table_name = 'contacts' AND merged_into = ?`, survivorContact).Scan(&n).Error)
	require.EqualValues(t, 2, n, "both losers must record their merge target")
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM contacts c JOIN centers ce ON ce.id = c.center_id
		 WHERE c.teacher_id <> ce.owner_id`).Scan(&n).Error)
	require.Zero(t, n, "every contact must anchor to the center owner")
}

// Class-scoped statements and runs coexist with the family/center-wide rows:
// the family unique keeps deduping only among class_id IS NULL rows, one
// class copy exists per (contact, period, class), one running run per period
// center-wide AND per (period, class) — and the down migration folds the
// schema back by deleting only the class-scoped rows.
func TestClassScopedStatementsAndRuns(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))

	db := openDB(t, url)
	f := seedNotificationParents(t, db, "+84900000017")
	classA, classB := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{classA, classB} {
		require.NoError(t, db.Exec(
			`INSERT INTO classes (id, teacher_id, center_id, name, start_date, default_unit_price)
			 VALUES (?, ?, ?, 'Lớp', '2026-01-05', 100000)`,
			id, f.teacherID, f.centerID).Error)
	}

	insertStatement := func(classID any, token string) error {
		return db.Exec(
			`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, class_id, token_hash, expires_at, total_due)
			 VALUES (?, ?, ?, ?, ?, ?, sha256(?::bytea), now() + interval '7 days', 50000)`,
			uuid.New(), f.teacherID, f.centerID, f.contactID, f.periodID, classID, token).Error
	}
	// The fixture already inserted the family statement for this contact and
	// period; a second family copy still collides.
	require.Error(t, insertStatement(nil, "fam-dup"),
		"the family unique must keep deduping class_id IS NULL rows")
	require.NoError(t, insertStatement(classA, "a1"),
		"a class copy must not collide with the family statement")
	require.NoError(t, insertStatement(classB, "b1"),
		"one copy per class of the same contact and period")
	require.Error(t, insertStatement(classA, "a2"),
		"a duplicate copy for the same class must be rejected")
	require.Error(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, class_id, token_hash, expires_at, total_due)
		 VALUES (?, ?, ?, ?, ?, ?, sha256('x'::bytea), now() + interval '7 days', 50000)`,
		uuid.New(), f.teacherID, f.centerID, f.contactID, f.periodID, uuid.New()).Error,
		"a class_id outside the center must fail the composite FK")

	// Each run needs its own sender: uq_notification_runs_one_active keeps
	// one running pass per sending device, so parallel class runs come from
	// different staff members.
	newMember := func(phone string) uuid.UUID {
		id := uuid.New()
		require.NoError(t, db.Exec(
			`INSERT INTO user_accounts (id, role, phone) VALUES (?, 'teachers', ?)`, id, phone).Error)
		// The teacher↔membership FKs are deferrable; insert the pair in one
		// transaction, the same shape the invitation flow uses.
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(
				`INSERT INTO teachers (id, full_name, center_id) VALUES (?, 'Học vụ', ?)`, id, f.centerID).Error; err != nil {
				return err
			}
			return tx.Exec(
				`INSERT INTO center_members (teacher_id, center_id) VALUES (?, ?)`, id, f.centerID).Error
		}))
		return id
	}
	hocVuA := newMember("+84900000117")
	hocVuB := newMember("+84900000217")
	hocVuC := newMember("+84900000317")

	insertRun := func(sender uuid.UUID, classID any) error {
		return db.Exec(
			`INSERT INTO notification_runs (id, teacher_id, center_id, billing_period_id, class_id, status)
			 VALUES (?, ?, ?, ?, ?, 'running')`,
			uuid.New(), sender, f.centerID, f.periodID, classID).Error
	}
	require.NoError(t, insertRun(f.teacherID, nil))
	require.Error(t, insertRun(hocVuA, nil),
		"one center-wide running run per period, as before")
	require.NoError(t, insertRun(hocVuA, classA),
		"a class run must coexist with the center-wide run of the same period")
	require.NoError(t, insertRun(hocVuB, classB),
		"two classes of the same period must run in parallel")
	require.Error(t, insertRun(hocVuC, classA),
		"the same class of the same period keeps the one-active-run conflict")

	// Down below 000017: class-scoped rows are deleted, family rows survive,
	// and the pre-class shape of both uniques is restored.
	require.NoError(t, m.Migrate(16))
	var statements, runs int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM statements`).Scan(&statements).Error)
	require.EqualValues(t, 1, statements, "only the family statement survives the down")
	require.NoError(t, db.Raw(`SELECT count(*) FROM notification_runs`).Scan(&runs).Error)
	require.EqualValues(t, 1, runs, "only the center-wide run survives the down")
	require.Error(t, db.Exec(
		`INSERT INTO notification_runs (id, teacher_id, center_id, billing_period_id, status)
		 VALUES (?, ?, ?, ?, 'running')`,
		uuid.New(), f.teacherID, f.centerID, f.periodID).Error,
		"the single-column running unique must be back after the down")
}

// The 000018 backfill preserves pre-catalog behavior over real legacy data:
// system roles and role-less live stints receive the operational baseline,
// legacy data.view_center_wide rows expand symmetrically into the twelve
// per-resource view_all keys, existing owner-written rows are never
// overwritten, and down removes exactly the recorded backfill rows — nothing
// the owner wrote or later changed.
func TestResourceActionCatalogBackfill(t *testing.T) {
	t.Parallel()
	url := startBarePostgres(t)

	m, err := database.NewMigrator(url)
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	require.NoError(t, database.MigrateUp(m))
	// Step back below 000018 and seed the pre-backfill shape.
	require.NoError(t, m.Migrate(17))

	db := openDB(t, url)
	live := seedNotificationParents(t, db, "+84900000801")
	roled := rbacMember(t, db, live.centerID, "+84900000802", false, false)
	roleless := rbacMember(t, db, live.centerID, "+84900000803", false, false)
	former := rbacMember(t, db, live.centerID, "+84900000804", false, true)

	// The raw center fixture bypasses the repository, so the system roles the
	// production path seeds must be inserted by hand.
	seedRoles := func(centerID uuid.UUID) {
		require.NoError(t, db.Exec(`
			INSERT INTO center_roles (id, center_id, key, name)
			VALUES (gen_random_uuid(), @cid, 'giao_vien', 'Giáo viên'),
				(gen_random_uuid(), @cid, 'hoc_vu', 'Học vụ'),
				(gen_random_uuid(), @cid, 'tro_giang', 'Trợ giảng')`,
			map[string]any{"cid": centerID}).Error)
	}
	seedRoles(live.centerID)
	roleID := func(centerID uuid.UUID, key string) uuid.UUID {
		var raw string
		require.NoError(t, db.Raw(
			`SELECT id FROM center_roles WHERE center_id = ? AND key = ?`,
			centerID, key).Scan(&raw).Error)
		id, err := uuid.Parse(raw)
		require.NoError(t, err)
		return id
	}
	gvRole := roleID(live.centerID, "giao_vien")
	hvRole := roleID(live.centerID, "hoc_vu")
	require.NoError(t, db.Exec(
		`UPDATE center_members SET role_id = ? WHERE teacher_id = ? AND center_id = ?`,
		gvRole, roled, live.centerID).Error)

	// Pre-existing assignments the backfill must respect: a manual role grant
	// that collides with a default, a legacy center-wide grant on a role and
	// on a member, a member's canonical deny, and a role-less member's legacy
	// deny.
	require.NoError(t, db.Exec(
		`INSERT INTO center_role_permissions (role_id, permission_key) VALUES (?, 'classes.create')`,
		gvRole).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO center_role_permissions (role_id, permission_key) VALUES (?, 'data.view_center_wide')`,
		hvRole).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		 VALUES (?, ?, 'data.view_center_wide', TRUE), (?, ?, 'students.view_all', FALSE)`,
		roled, live.centerID, roled, live.centerID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		 VALUES (?, ?, 'data.view_center_wide', FALSE)`,
		roleless, live.centerID).Error)

	// A retired center's roles must be skipped entirely.
	retired := seedNotificationParents(t, db, "+84900000805")
	seedRoles(retired.centerID)
	require.NoError(t, db.Exec(
		`UPDATE centers SET deleted_at = now() WHERE id = ?`, retired.centerID).Error)

	require.NoError(t, database.MigrateUp(m))

	defaults := authctx.DefaultRoleKeys()
	var n int64
	// Every live-center system role holds the full baseline; the colliding
	// manual grant is not duplicated.
	rolePermCount := func(id uuid.UUID) int64 {
		var c int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM center_role_permissions WHERE role_id = ?`, id).Scan(&c).Error)
		return c
	}
	require.EqualValues(t, len(defaults), rolePermCount(gvRole),
		"giao_vien holds exactly the baseline — the manual classes.create must not duplicate")
	require.EqualValues(t, len(defaults)+10, rolePermCount(hvRole),
		"hoc_vu holds baseline + ten enforced view_all expansions — the alias and the never-enforced scope keys are retired")
	require.EqualValues(t, len(defaults), rolePermCount(roleID(live.centerID, "tro_giang")))
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM center_role_permissions rp
		 JOIN center_roles cr ON cr.id = rp.role_id
		 WHERE cr.center_id = ?`, retired.centerID).Scan(&n).Error)
	require.Zero(t, n, "a retired center's roles receive nothing")
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM center_role_permissions
		 WHERE role_id = ? AND permission_key IN ('members.manage', 'dashboard.view', 'reports.send')`,
		gvRole).Scan(&n).Error)
	require.Zero(t, n, "legacy identity keys must never be granted by default")

	// The roled member gets no member-level defaults; their legacy grant
	// expands to eleven TRUE view_all rows while the pre-existing canonical
	// deny survives untouched.
	memberRows := func(teacherID uuid.UUID) map[string]bool {
		var rows []struct {
			PermissionKey string
			Allowed       bool
		}
		require.NoError(t, db.Raw(
			`SELECT permission_key, allowed FROM center_member_permissions
			 WHERE teacher_id = ? AND center_id = ?`, teacherID, live.centerID).Scan(&rows).Error)
		out := map[string]bool{}
		for _, r := range rows {
			out[r.PermissionKey] = r.Allowed
		}
		return out
	}
	roledRows := memberRows(roled)
	require.Len(t, roledRows, 10, "ten enforced view_all expansions, no defaults for a roled member")
	require.NotContains(t, roledRows, "data.view_center_wide",
		"the alias row is retired — its expansion already materialized the canonical keys")
	require.True(t, roledRows["classes.view_all"])
	require.False(t, roledRows["students.view_all"], "the owner's canonical deny must survive the expansion")

	// The role-less live member gets the baseline as grants plus the
	// symmetric deny expansion of their legacy deny.
	rolelessRows := memberRows(roleless)
	require.Len(t, rolelessRows, len(defaults)+10)
	for _, key := range defaults {
		require.Truef(t, rolelessRows[key], "role-less member must hold default %s", key)
	}
	require.NotContains(t, rolelessRows, "data.view_center_wide", "the alias deny row is retired")
	require.False(t, rolelessRows["students.view_all"], "a legacy deny expands into per-resource denies")

	// Closed stints and the owner stay untouched.
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM center_member_permissions WHERE teacher_id IN (?, ?)`,
		former, live.teacherID).Scan(&n).Error)
	require.Zero(t, n)

	// Effective-access parity through the real resolver algebra: the roled
	// member sees center-wide classes but keeps the denied students scope.
	perms := authctx.BuildPermSet(
		nil,
		[]string{"data.view_center_wide", "classes.view_all"},
		[]string{"students.view_all"})
	require.True(t, perms.HasKey("classes.view_all"))
	require.False(t, perms.HasKey("students.view_all"))

	// CAS anchor columns arrive at version 1.
	var v int64
	require.NoError(t, db.Raw(
		`SELECT assignment_version FROM center_roles WHERE id = ?`, gvRole).Scan(&v).Error)
	require.EqualValues(t, 1, v)
	require.NoError(t, db.Raw(
		`SELECT assignment_version FROM center_members WHERE teacher_id = ? AND center_id = ?`,
		roled, live.centerID).Scan(&v).Error)
	require.EqualValues(t, 1, v)

	// The ledger records the mapping checksum and exact per-step counts.
	var ledger struct {
		MappingChecksum   string
		RoleDefaultRows   int
		MemberDefaultRows int
		ScopeRoleRows     int
		ScopeMemberRows   int
	}
	require.NoError(t, db.Raw(`SELECT * FROM rbac_backfill_ledger`).Scan(&ledger).Error)
	require.Regexp(t, `^sha256:[0-9a-f]{64}$`, ledger.MappingChecksum)
	require.Equal(t, 3*len(defaults)-1, ledger.RoleDefaultRows,
		"three system roles minus the one colliding manual grant")
	require.Equal(t, len(defaults), ledger.MemberDefaultRows)
	require.Equal(t, 12, ledger.ScopeRoleRows)
	require.Equal(t, 11+12, ledger.ScopeMemberRows,
		"eleven for the roled member (deny collision) plus twelve for the role-less deny")

	// Owner decisions made after the backfill must survive down: flip one
	// expanded deny to a grant. Down then removes only recorded, still-matching
	// rows — pre-existing assignments and the flipped row stay.
	require.NoError(t, db.Exec(
		`UPDATE center_member_permissions SET allowed = TRUE
		 WHERE teacher_id = ? AND center_id = ? AND permission_key = 'students.view_all'`,
		roleless, live.centerID).Error)
	require.NoError(t, m.Migrate(17))

	require.EqualValues(t, 1, rolePermCount(gvRole), "only the manual classes.create survives down")
	require.Zero(t, rolePermCount(hvRole),
		"nothing survives down: the backfill rows are removed and the retired alias row is deliberately not rebuilt")
	roledRows = memberRows(roled)
	require.Len(t, roledRows, 1, "only the pre-existing canonical deny survives down")
	require.False(t, roledRows["students.view_all"])
	rolelessRows = memberRows(roleless)
	require.Len(t, rolelessRows, 1)
	require.True(t, rolelessRows["students.view_all"], "the owner-flipped row must not be deleted by down")

	tables := tableNames(t, db)
	require.False(t, tables["rbac_backfill_rows"])
	require.False(t, tables["rbac_backfill_ledger"])
	cols := nameSet(t, db,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'center_roles'`)
	require.False(t, cols["assignment_version"], "down must drop the CAS column")
}
