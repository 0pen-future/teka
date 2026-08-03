// Package seeds populates development data. Runs are idempotent: teachers are
// keyed by phone, roster data by the owning teacher having none yet, and
// existing rows are never modified, so reseeding a database with real data is
// safe.
package seeds

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"teka/apps/api/internal/shared/id"
)

const (
	bcryptCost      = 12
	defaultTimezone = "Asia/Ho_Chi_Minh"
)

type seedTeacher struct {
	Phone    string
	Password string
	FullName string
}

// Development credentials only — never used outside seeded local databases.
var seedTeachers = []seedTeacher{
	{Phone: "+84901000001", Password: "lan-password", FullName: "Cô Lan"},
	{Phone: "+84901000002", Password: "minh-password", FullName: "Thầy Minh"},
}

type seedContact struct {
	Phone    string
	FullName string
}

// Roster demo data hangs off the first seed teacher (Cô Lan).
var seedContacts = []seedContact{
	{Phone: "+84912000001", FullName: "Chị Hoa"},
	{Phone: "+84912000002", FullName: "Anh Tuấn"},
	{Phone: "+84912000003", FullName: "Chị Mai"},
	{Phone: "+84912000004", FullName: "Bác Hùng"},
}

type seedStudent struct {
	FullName     string
	ContactPhone string // resolves to the seeded contact's id at insert time
	DisplayNote  string
}

// Students hang off the seeded contacts; Chị Hoa has two children so the
// attendance-sheet disambiguation note has data to show.
var seedStudents = []seedStudent{
	{FullName: "Bé An", ContactPhone: "+84912000001", DisplayNote: "Con chị Hoa - lớp 8"},
	{FullName: "Bé Bình", ContactPhone: "+84912000001", DisplayNote: "Con chị Hoa - lớp 9"},
	{FullName: "Bé Cường", ContactPhone: "+84912000002"},
	{FullName: "Bé Dung", ContactPhone: "+84912000003"},
}

type seedSchedule struct {
	Weekday     int16 // 0 = Chủ nhật, matches int(time.Sunday)
	StartTime   string
	DurationMin int16
}

type seedClass struct {
	Name      string
	StartDate string // YYYY-MM-DD
	UnitPrice int64  // đồng per session
	Schedules []seedSchedule
}

type seedEnrollment struct {
	StudentName string
	ClassName   string
	StartedOn   string // YYYY-MM-DD
	EndedOn     string // "" = still enrolled
}

// The three enrollment shapes plans 03 and 04 develop against: joined on the
// class start date, joined mid-month (the product's whole reason for
// existing), and already departed. unit_price is copied from the class at
// insert, exactly as the service does.
var seedEnrollments = []seedEnrollment{
	{StudentName: "Bé An", ClassName: "Toán 8 - Tối Thứ Ba", StartedOn: "2026-01-06"},
	{StudentName: "Bé Bình", ClassName: "Toán 8 - Tối Thứ Ba", StartedOn: "2026-01-20"},
	{StudentName: "Bé Cường", ClassName: "Toán 8 - Tối Thứ Ba", StartedOn: "2026-01-06", EndedOn: "2026-03-31"},
	{StudentName: "Bé Dung", ClassName: "Văn 9 - Sáng Thứ Bảy", StartedOn: "2026-02-07"},
}

// Two classes on different weekdays with different opening dates, so session
// generation and billing previews have varied timetables to work against.
var seedClasses = []seedClass{
	{
		Name:      "Toán 8 - Tối Thứ Ba",
		StartDate: "2026-01-06",
		UnitPrice: 150_000,
		Schedules: []seedSchedule{
			{Weekday: 2, StartTime: "18:00", DurationMin: 90},
		},
	},
	{
		Name:      "Văn 9 - Sáng Thứ Bảy",
		StartDate: "2026-02-07",
		UnitPrice: 200_000,
		Schedules: []seedSchedule{
			{Weekday: 6, StartTime: "09:00", DurationMin: 120},
		},
	},
}

// Run inserts the seed teachers that do not exist yet, then demo roster data
// for the first teacher, and reports each outcome.
func Run(ctx context.Context, db *gorm.DB, log *slog.Logger) error {
	teacherIDs := make([]uuid.UUID, 0, len(seedTeachers))
	for _, s := range seedTeachers {
		teacherID, err := ensureTeacher(ctx, db, log, s)
		if err != nil {
			return err
		}
		teacherIDs = append(teacherIDs, teacherID)
	}
	if err := seedRoster(ctx, db, log, teacherIDs[0]); err != nil {
		return err
	}
	if err := seedStudentList(ctx, db, log, teacherIDs[0]); err != nil {
		return err
	}
	if err := seedClassList(ctx, db, log, teacherIDs[0]); err != nil {
		return err
	}
	return seedEnrollmentList(ctx, db, log, teacherIDs[0])
}

// ensureTeacher returns the id of the teacher with s.Phone, creating the
// user_accounts + teachers row pair in one transaction when absent.
func ensureTeacher(ctx context.Context, db *gorm.DB, log *slog.Logger, s seedTeacher) (uuid.UUID, error) {
	var existing []uuid.UUID
	err := db.WithContext(ctx).
		Raw("SELECT id FROM user_accounts WHERE phone = ? AND deleted_at IS NULL", s.Phone).
		Scan(&existing).Error
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("seed: look up %s: %w", s.Phone, err)
	}
	if len(existing) > 0 {
		log.Info("seed: teacher exists, skipping", "phone", s.Phone)
		return existing[0], nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(s.Password), bcryptCost)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("seed: hash password for %s: %w", s.Phone, err)
	}
	accountID := id.New()
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"INSERT INTO user_accounts (id, role, phone, password_hash, status) VALUES (?, 'teachers', ?, ?, 'active')",
			accountID, s.Phone, string(hash),
		).Error; err != nil {
			return err
		}
		return tx.Exec(
			"INSERT INTO teachers (id, full_name, timezone) VALUES (?, ?, ?)",
			accountID, s.FullName, defaultTimezone,
		).Error
	})
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("seed: create %s: %w", s.Phone, err)
	}
	log.Info("seed: teacher created", "phone", s.Phone, "full_name", s.FullName)
	return accountID, nil
}

// seedRoster inserts demo contacts for one teacher. It is all-or-nothing per
// teacher: any pre-existing contact means the roster was seeded (or the
// teacher has real data) and the whole block is skipped.
func seedRoster(ctx context.Context, db *gorm.DB, log *slog.Logger, teacherID uuid.UUID) error {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT count(*) FROM contacts WHERE teacher_id = ? AND deleted_at IS NULL", teacherID).
		Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up contacts: %w", err)
	}
	if count > 0 {
		log.Info("seed: roster exists, skipping", "teacher_id", teacherID)
		return nil
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, c := range seedContacts {
			if err := tx.Exec(
				"INSERT INTO contacts (id, teacher_id, full_name, phone) VALUES (?, ?, ?, ?)",
				id.New(), teacherID, c.FullName, c.Phone,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("seed: create roster: %w", err)
	}
	log.Info("seed: roster created", "teacher_id", teacherID, "contacts", len(seedContacts))
	return nil
}

// seedStudentList inserts the demo students against the seeded contacts,
// skipped wholesale when the teacher already has any student. Contacts are
// resolved by phone so the block also works against a roster seeded earlier.
func seedStudentList(ctx context.Context, db *gorm.DB, log *slog.Logger, teacherID uuid.UUID) error {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT count(*) FROM students WHERE teacher_id = ? AND deleted_at IS NULL", teacherID).
		Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up students: %w", err)
	}
	if count > 0 {
		log.Info("seed: students exist, skipping", "teacher_id", teacherID)
		return nil
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, s := range seedStudents {
			var contactIDs []uuid.UUID
			if err := tx.Raw(
				"SELECT id FROM contacts WHERE teacher_id = ? AND phone = ? AND deleted_at IS NULL",
				teacherID, s.ContactPhone,
			).Scan(&contactIDs).Error; err != nil {
				return err
			}
			if len(contactIDs) == 0 {
				return fmt.Errorf("no seeded contact with phone %s for student %s", s.ContactPhone, s.FullName)
			}
			var note any
			if s.DisplayNote != "" {
				note = s.DisplayNote
			}
			if err := tx.Exec(
				"INSERT INTO students (id, teacher_id, contact_id, full_name, display_note) VALUES (?, ?, ?, ?, ?)",
				id.New(), teacherID, contactIDs[0], s.FullName, note,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("seed: create students: %w", err)
	}
	log.Info("seed: students created", "teacher_id", teacherID, "students", len(seedStudents))
	return nil
}

// seedClassList inserts the demo classes with their weekly schedules for one
// teacher, skipped wholesale when the teacher already has any class.
func seedClassList(ctx context.Context, db *gorm.DB, log *slog.Logger, teacherID uuid.UUID) error {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT count(*) FROM classes WHERE teacher_id = ? AND deleted_at IS NULL", teacherID).
		Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up classes: %w", err)
	}
	if count > 0 {
		log.Info("seed: classes exist, skipping", "teacher_id", teacherID)
		return nil
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, c := range seedClasses {
			classID := id.New()
			if err := tx.Exec(
				"INSERT INTO classes (id, teacher_id, name, start_date, default_unit_price, status) VALUES (?, ?, ?, ?::date, ?, 'active')",
				classID, teacherID, c.Name, c.StartDate, c.UnitPrice,
			).Error; err != nil {
				return err
			}
			for _, s := range c.Schedules {
				if err := tx.Exec(
					"INSERT INTO class_schedules (id, teacher_id, class_id, weekday, start_time, duration_min, effective_from) VALUES (?, ?, ?, ?, ?::time, ?, ?::date)",
					id.New(), teacherID, classID, s.Weekday, s.StartTime, s.DurationMin, c.StartDate,
				).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("seed: create classes: %w", err)
	}
	log.Info("seed: classes created", "teacher_id", teacherID, "classes", len(seedClasses))
	return nil
}

// seedEnrollmentList enrolls the seeded students into the seeded classes,
// skipped wholesale when the teacher already has any enrollment. Students and
// classes are resolved by name; unit_price is copied from the class's current
// default, matching what the enrollments service does.
func seedEnrollmentList(ctx context.Context, db *gorm.DB, log *slog.Logger, teacherID uuid.UUID) error {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT count(*) FROM enrollments WHERE teacher_id = ? AND deleted_at IS NULL", teacherID).
		Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up enrollments: %w", err)
	}
	if count > 0 {
		log.Info("seed: enrollments exist, skipping", "teacher_id", teacherID)
		return nil
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, e := range seedEnrollments {
			var studentIDs []uuid.UUID
			if err := tx.Raw(
				"SELECT id FROM students WHERE teacher_id = ? AND full_name = ? AND deleted_at IS NULL",
				teacherID, e.StudentName,
			).Scan(&studentIDs).Error; err != nil {
				return err
			}
			if len(studentIDs) == 0 {
				return fmt.Errorf("no seeded student named %s", e.StudentName)
			}
			type classRow struct {
				ID               uuid.UUID
				DefaultUnitPrice int64
			}
			var classRows []classRow
			if err := tx.Raw(
				"SELECT id, default_unit_price FROM classes WHERE teacher_id = ? AND name = ? AND deleted_at IS NULL",
				teacherID, e.ClassName,
			).Scan(&classRows).Error; err != nil {
				return err
			}
			if len(classRows) == 0 {
				return fmt.Errorf("no seeded class named %s", e.ClassName)
			}
			var endedOn any
			if e.EndedOn != "" {
				endedOn = e.EndedOn
			}
			if err := tx.Exec(
				"INSERT INTO enrollments (id, teacher_id, student_id, class_id, started_on, ended_on, unit_price) VALUES (?, ?, ?, ?, ?::date, ?::date, ?)",
				id.New(), teacherID, studentIDs[0], classRows[0].ID, e.StartedOn, endedOn, classRows[0].DefaultUnitPrice,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("seed: create enrollments: %w", err)
	}
	log.Info("seed: enrollments created", "teacher_id", teacherID, "enrollments", len(seedEnrollments))
	return nil
}
