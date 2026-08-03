package testutil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/features/enrollments"
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

// Teacher inserts a user_accounts + teachers row pair directly (bypassing the
// service) and returns both. Passwords hash at bcrypt.MinCost so fixtures stay
// fast; phones default to a unique random +84 number so tests never collide.
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
	if err := db.Create(acct).Error; err != nil {
		t.Fatalf("insert fixture account %s: %v", acct.Phone, err)
	}
	if err := db.Create(teacher).Error; err != nil {
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
