package testutil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// DefaultPassword is the plaintext behind every fixture account's hash unless
// WithPassword overrides it.
const DefaultPassword = "password-123"

// JWTSecret signs access tokens in integration tests; shared so tests can mint
// tokens against the same key the service under test verifies with.
const JWTSecret = "integration-test-secret-0123456789abcdef"

// TeacherOption customizes a fixture teacher before insertion.
type TeacherOption func(acct *teachers.Account, t *teachers.Teacher, password *string)

// WithPhone sets the fixture account's phone (stored verbatim — pass E.164).
func WithPhone(phone string) TeacherOption {
	return func(acct *teachers.Account, _ *teachers.Teacher, _ *string) { acct.Phone = phone }
}

// WithFullName sets the fixture teacher's display name.
func WithFullName(name string) TeacherOption {
	return func(_ *teachers.Account, t *teachers.Teacher, _ *string) { t.FullName = name }
}

// WithStatus sets the fixture account's status.
func WithStatus(status string) TeacherOption {
	return func(acct *teachers.Account, _ *teachers.Teacher, _ *string) { acct.Status = status }
}

// WithPassword sets the plaintext the stored hash is derived from.
func WithPassword(plaintext string) TeacherOption {
	return func(_ *teachers.Account, _ *teachers.Teacher, pw *string) { *pw = plaintext }
}

// Teacher inserts a fixture teacher directly (bypassing the service): their
// personal centers row, the user_accounts + teachers pair, and the live
// center_members row — one transaction, because the center's owner FK is
// deferred until the teachers row exists. The teacher's center is available
// as the returned Teacher.CenterID. Passwords hash at bcrypt.MinCost so
// fixtures stay fast; phones default to a unique random +84 number so tests
// never collide.
func Teacher(t *testing.T, db *gorm.DB, opts ...TeacherOption) (*teachers.Account, *teachers.Teacher) {
	t.Helper()
	accountID := id.New()
	acct := &teachers.Account{
		ID:     accountID,
		Role:   authctx.RoleTeacher,
		Phone:  randomPhone(),
		Status: teachers.StatusActive,
	}
	teacher := &teachers.Teacher{
		ID:       accountID,
		FullName: "Fixture Teacher",
		Timezone: teachers.DefaultTimezone,
		CenterID: id.New(),
	}
	password := DefaultPassword
	for _, opt := range opts {
		opt(acct, teacher, &password)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	hashStr := string(hash)
	acct.PasswordHash = &hashStr
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&centers.Center{
			ID:      teacher.CenterID,
			Name:    teacher.FullName,
			OwnerID: accountID,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(acct).Error; err != nil {
			return err
		}
		if err := tx.Create(teacher).Error; err != nil {
			return err
		}
		return tx.Exec(
			"INSERT INTO center_members (teacher_id, center_id) VALUES (?, ?)",
			accountID, teacher.CenterID).Error
	})
	if err != nil {
		t.Fatalf("insert fixture teacher %s: %v", acct.Phone, err)
	}
	return acct, teacher
}

// randomPhone derives a valid-shaped, effectively unique +849xxxxxxxx number
// from random UUID bytes.
func randomPhone() string {
	u := uuid.New()
	return fmt.Sprintf("+849%08d", binary.BigEndian.Uint32(u[0:4])%100000000)
}

// centerOf resolves the teacher's current center; business fixtures anchor
// their rows in it the same way the scoped services do.
func centerOf(t *testing.T, db *gorm.DB, teacherID uuid.UUID) uuid.UUID {
	t.Helper()
	// Scanning straight into a bare uuid.UUID skips its sql.Scanner and hits
	// GORM's element-wise array path instead; wrap it in a struct field, the
	// same shape ScopeFor scans into.
	var row struct{ CenterID uuid.UUID }
	err := db.Raw("SELECT center_id FROM teachers WHERE id = ?", teacherID).Scan(&row).Error
	if err != nil {
		t.Fatalf("resolve fixture teacher %s center: %v", teacherID, err)
	}
	if row.CenterID == uuid.Nil {
		t.Fatalf("fixture teacher %s has no center row", teacherID)
	}
	return row.CenterID
}

// ScopeFor resolves the teacher's live scope straight from the database the
// way the scope middleware does, so service-level tests call scoped services
// with the same tenant context a request would carry.
func ScopeFor(t *testing.T, db *gorm.DB, teacherID uuid.UUID) authctx.Scope {
	t.Helper()
	var row struct {
		CenterID       uuid.UUID
		IsOwner        bool
		CanSendReports bool
	}
	err := db.Raw(`
		SELECT t.center_id, (c.owner_id = t.id) AS is_owner,
			COALESCE(cm.can_send_reports, FALSE) AS can_send_reports
		FROM teachers t
		JOIN centers c ON c.id = t.center_id
		LEFT JOIN center_members cm ON cm.teacher_id = t.id
			AND cm.center_id = t.center_id AND cm.left_at IS NULL
		WHERE t.id = ?`, teacherID).Scan(&row).Error
	if err != nil {
		t.Fatalf("resolve fixture scope for %s: %v", teacherID, err)
	}
	if row.CenterID == uuid.Nil {
		t.Fatalf("fixture teacher %s has no center row", teacherID)
	}
	return authctx.Scope{
		TeacherID:      teacherID,
		CenterID:       row.CenterID,
		IsOwner:        row.IsOwner,
		CanSendReports: row.CanSendReports,
	}
}

// JoinCenter moves the teacher into the target center directly (bypassing the
// service): closes their live membership stint, opens one in the target,
// re-points teachers.center_id, and retires their vacated personal center.
func JoinCenter(t *testing.T, db *gorm.DB, teacherID, centerID uuid.UUID) {
	t.Helper()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"UPDATE center_members SET left_at = now() WHERE teacher_id = ? AND left_at IS NULL",
			teacherID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO center_members (teacher_id, center_id) VALUES (?, ?)
			ON CONFLICT (teacher_id, center_id) DO UPDATE SET left_at = NULL, joined_at = now()`,
			teacherID, centerID).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			"UPDATE teachers SET center_id = ? WHERE id = ?", centerID, teacherID).Error; err != nil {
			return err
		}
		return tx.Exec(
			"UPDATE centers SET deleted_at = now() WHERE owner_id = ? AND deleted_at IS NULL",
			teacherID).Error
	})
	if err != nil {
		t.Fatalf("move fixture teacher %s into center %s: %v", teacherID, centerID, err)
	}
}

// GrantSendReports sets the teacher's can_send_reports flag on their live
// membership directly (bypassing the owner grant flow), so authorization
// tests can make — or revoke — a delegated report sender in one call.
func GrantSendReports(t *testing.T, db *gorm.DB, teacherID uuid.UUID, granted bool) {
	t.Helper()
	res := db.Exec(
		"UPDATE center_members SET can_send_reports = ? WHERE teacher_id = ? AND left_at IS NULL",
		granted, teacherID)
	if res.Error != nil {
		t.Fatalf("set can_send_reports=%v for fixture teacher %s: %v", granted, teacherID, res.Error)
	}
	if res.RowsAffected == 0 {
		t.Fatalf("fixture teacher %s has no live membership to flag", teacherID)
	}
}

// Secretary creates a member teacher in centerID already holding
// can_send_reports — the delegated report sender the authorization matrix
// revolves around.
func Secretary(t *testing.T, db *gorm.DB, centerID uuid.UUID) (*teachers.Account, *teachers.Teacher) {
	account, teacher := Teacher(t, db)
	JoinCenter(t, db, teacher.ID, centerID)
	GrantSendReports(t, db, teacher.ID, true)
	return account, teacher
}

// ContactOption customizes a fixture contact before insertion.
type ContactOption func(c *contacts.Contact)

// WithContactFullName sets the fixture contact's display name.
func WithContactFullName(name string) ContactOption {
	return func(c *contacts.Contact) { c.FullName = name }
}

// WithContactPhone sets the fixture contact's phone (stored verbatim — pass E.164).
func WithContactPhone(phone string) ContactOption {
	return func(c *contacts.Contact) { c.Phone = phone }
}

// Contact inserts a contacts row for the teacher directly (bypassing the
// service); phones default to a unique random +84 number so tests never collide.
func Contact(t *testing.T, db *gorm.DB, teacherID uuid.UUID, opts ...ContactOption) *contacts.Contact {
	t.Helper()
	c := &contacts.Contact{
		ID:        id.New(),
		TeacherID: teacherID,
		CenterID:  centerOf(t, db, teacherID),
		FullName:  "Fixture Contact",
		Phone:     randomPhone(),
	}
	for _, opt := range opts {
		opt(c)
	}
	if err := db.Create(c).Error; err != nil {
		t.Fatalf("insert fixture contact %s: %v", c.Phone, err)
	}
	return c
}

// Enrollment inserts an enrollments row directly (bypassing the service), so
// tests control the unit price and start date exactly. Defaults: 100 000 đồng,
// open-ended.
func Enrollment(t *testing.T, db *gorm.DB, teacherID, studentID, classID uuid.UUID, startedOn time.Time) *enrollments.Enrollment {
	t.Helper()
	e := &enrollments.Enrollment{
		ID:        id.New(),
		TeacherID: teacherID,
		CenterID:  centerOf(t, db, teacherID),
		StudentID: studentID,
		ClassID:   classID,
		StartedOn: startedOn,
		UnitPrice: 100_000,
	}
	if err := db.Create(e).Error; err != nil {
		t.Fatalf("insert fixture enrollment: %v", err)
	}
	return e
}

// StudentOption customizes a fixture student before insertion.
type StudentOption func(s *students.Student)

// WithStudentFullName sets the fixture student's display name.
func WithStudentFullName(name string) StudentOption {
	return func(s *students.Student) { s.FullName = name }
}

// WithStudentDisplayNote sets the fixture student's attendance-sheet note.
func WithStudentDisplayNote(note string) StudentOption {
	return func(s *students.Student) { s.DisplayNote = &note }
}

// Student inserts a students row for the teacher directly (bypassing the
// service). The contact must belong to the same teacher — the composite FK
// rejects the insert otherwise.
func Student(t *testing.T, db *gorm.DB, teacherID, contactID uuid.UUID, opts ...StudentOption) *students.Student {
	t.Helper()
	s := &students.Student{
		ID:        id.New(),
		TeacherID: teacherID,
		CenterID:  centerOf(t, db, teacherID),
		ContactID: contactID,
		FullName:  "Fixture Student",
	}
	for _, opt := range opts {
		opt(s)
	}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("insert fixture student %s: %v", s.FullName, err)
	}
	return s
}

// ClassOption customizes a fixture class before insertion.
type ClassOption func(c *classes.Class)

// WithClassName sets the fixture class's name.
func WithClassName(name string) ClassOption {
	return func(c *classes.Class) { c.Name = name }
}

// WithClassStartDate sets the fixture class's opening date.
func WithClassStartDate(d time.Time) ClassOption {
	return func(c *classes.Class) { c.StartDate = d }
}

// WithClassStatus sets the fixture class's status.
func WithClassStatus(status string) ClassOption {
	return func(c *classes.Class) { c.Status = status }
}

// WithClassUnitPrice sets the fixture class's default unit price in đồng.
func WithClassUnitPrice(price int64) ClassOption {
	return func(c *classes.Class) { c.DefaultUnitPrice = price }
}

// Class inserts a classes row for the teacher directly (bypassing the
// service). Defaults: active, opens 2026-01-05, 100 000 đồng per session.
func Class(t *testing.T, db *gorm.DB, teacherID uuid.UUID, opts ...ClassOption) *classes.Class {
	t.Helper()
	c := &classes.Class{
		ID:               id.New(),
		TeacherID:        teacherID,
		CenterID:         centerOf(t, db, teacherID),
		Name:             "Fixture Class",
		StartDate:        time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		DefaultUnitPrice: 100_000,
		Status:           classes.StatusActive,
	}
	for _, opt := range opts {
		opt(c)
	}
	if err := db.Omit("Schedules").Create(c).Error; err != nil {
		t.Fatalf("insert fixture class %s: %v", c.Name, err)
	}
	return c
}

// Schedule inserts a class_schedules row for the class directly. The row's
// effective_from defaults to the class start date and stays open-ended.
func Schedule(t *testing.T, db *gorm.DB, class *classes.Class, weekday int16, startTime string) *classes.Schedule {
	t.Helper()
	s := &classes.Schedule{
		ID:            id.New(),
		TeacherID:     class.TeacherID,
		CenterID:      class.CenterID,
		ClassID:       class.ID,
		Weekday:       weekday,
		StartTime:     classes.TimeOfDay(startTime),
		DurationMin:   90,
		EffectiveFrom: class.StartDate,
	}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("insert fixture schedule for class %s: %v", class.Name, err)
	}
	return s
}

// SessionOption customizes a fixture session before insertion.
type SessionOption func(s *sessions.Session)

// WithSessionStatus sets the fixture session's status.
func WithSessionStatus(status string) SessionOption {
	return func(s *sessions.Session) { s.Status = status }
}

// WithSessionStartTime sets the fixture session's start_time.
func WithSessionStartTime(hhmm string) SessionOption {
	return func(s *sessions.Session) {
		t := classes.TimeOfDay(hhmm)
		s.StartTime = &t
	}
}

// WithSessionCancelReason sets the fixture session's cancel_reason.
func WithSessionCancelReason(reason string) SessionOption {
	return func(s *sessions.Session) { s.CancelReason = &reason }
}

// WithSessionAttendanceConfirmed stamps attendance_confirmed_at and flips
// status to held, simulating a confirmed session ahead of phase 2's
// attendance endpoint existing.
func WithSessionAttendanceConfirmed(at time.Time) SessionOption {
	return func(s *sessions.Session) {
		s.AttendanceConfirmedAt = &at
		s.Status = sessions.StatusHeld
	}
}

// Session inserts a class_sessions row for the class directly (bypassing the
// service). Defaults: planned, no start_time. The class must belong to the
// same teacher — the composite FK rejects the insert otherwise.
func Session(t *testing.T, db *gorm.DB, teacherID, classID uuid.UUID, date time.Time, opts ...SessionOption) *sessions.Session {
	t.Helper()
	s := &sessions.Session{
		ID:          id.New(),
		TeacherID:   teacherID,
		CenterID:    centerOf(t, db, teacherID),
		ClassID:     classID,
		SessionDate: date,
		Status:      sessions.StatusPlanned,
	}
	for _, opt := range opts {
		opt(s)
	}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("insert fixture session for class %s: %v", classID, err)
	}
	return s
}

// AttendanceOption customizes a fixture attendance record before insertion.
type AttendanceOption func(r *attendance.Record)

// WithAttendanceStatus sets the fixture record's status.
func WithAttendanceStatus(status string) AttendanceOption {
	return func(r *attendance.Record) { r.Status = status }
}

// WithAttendanceBillable sets the fixture record's billable flag.
func WithAttendanceBillable(billable bool) AttendanceOption {
	return func(r *attendance.Record) { r.Billable = billable }
}

// WithAttendanceNote sets the fixture record's note.
func WithAttendanceNote(note string) AttendanceOption {
	return func(r *attendance.Record) { r.Note = &note }
}

// WithAttendanceRecordedAt overrides the fixture record's recorded_at, so
// tests can assert it is preserved across a later re-confirm.
func WithAttendanceRecordedAt(at time.Time) AttendanceOption {
	return func(r *attendance.Record) { r.RecordedAt = at }
}

// AttendanceRecord inserts an attendance_records row directly (bypassing the
// service). Defaults: present, billable. The session, student, and
// enrollment must all belong to the same teacher — the composite FKs reject
// the insert otherwise.
func AttendanceRecord(t *testing.T, db *gorm.DB, teacherID, sessionID, studentID, enrollmentID uuid.UUID, opts ...AttendanceOption) *attendance.Record {
	t.Helper()
	now := time.Now()
	r := &attendance.Record{
		ID:           id.New(),
		TeacherID:    teacherID,
		CenterID:     centerOf(t, db, teacherID),
		SessionID:    sessionID,
		StudentID:    studentID,
		EnrollmentID: enrollmentID,
		Status:       attendance.StatusPresent,
		Billable:     true,
		RecordedAt:   now,
		UpdatedAt:    now,
	}
	for _, opt := range opts {
		opt(r)
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("insert fixture attendance record for session %s: %v", sessionID, err)
	}
	return r
}
