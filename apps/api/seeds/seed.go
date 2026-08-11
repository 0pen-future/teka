// Package seeds populates development data. Runs are idempotent: teachers are
// keyed by phone, roster data by the owning teacher having none yet, and
// existing rows are never modified, so reseeding a database with real data is
// safe.
package seeds

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/authctx"
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
// attendance-sheet disambiguation note has data to show, and Chị Mai has two
// children in the same class so a public statement can show a multi-child
// family whose invoices stay unpaid even after Chị Hoa's are settled.
var seedStudents = []seedStudent{
	{FullName: "Bé An", ContactPhone: "+84912000001", DisplayNote: "Con chị Hoa - lớp 8"},
	{FullName: "Bé Bình", ContactPhone: "+84912000001", DisplayNote: "Con chị Hoa - lớp 9"},
	{FullName: "Bé Cường", ContactPhone: "+84912000002"},
	{FullName: "Bé Dung", ContactPhone: "+84912000003"},
	{FullName: "Bé Em", ContactPhone: "+84912000003"},
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
	{StudentName: "Bé Em", ClassName: "Văn 9 - Sáng Thứ Bảy", StartedOn: "2026-02-07"},
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
	if err := seedEnrollmentList(ctx, db, log, teacherIDs[0]); err != nil {
		return err
	}
	return seedSessionList(ctx, db, log, teacherIDs[0])
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

// pendingAttendanceCount is how many of the most recent past sessions are
// left unconfirmed, so phase 3's pending-attendance warning feed has
// something to warn about from the moment the database is seeded.
const pendingAttendanceCount = 2

// pastSession is one generated session that fell on or before "now", tracked
// across all seeded classes so confirmation order is a single date-sorted
// timeline rather than per-class.
type pastSession struct {
	ID      uuid.UUID
	ClassID uuid.UUID
	Date    time.Time
}

// seedSessionList generates class sessions for the seeded classes across the
// previous and current calendar month, through the real sessions.Service —
// exercising the same generation path the API uses rather than hand-rolled
// inserts. Skipped wholesale when the teacher already has any session, so
// reseeding never duplicates rows. Attendance is then confirmed, through the
// real attendance.Service, for every past session except the
// pendingAttendanceCount most recent — a deterministic scatter of absences
// gives the seeded data realistic variation without needing true randomness.
func seedSessionList(ctx context.Context, db *gorm.DB, log *slog.Logger, teacherID uuid.UUID) error {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT count(*) FROM class_sessions WHERE teacher_id = ? AND deleted_at IS NULL", teacherID).
		Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up sessions: %w", err)
	}
	if count > 0 {
		log.Info("seed: sessions exist, skipping", "teacher_id", teacherID)
		return nil
	}

	var classIDs []uuid.UUID
	if err := db.WithContext(ctx).
		Raw("SELECT id FROM classes WHERE teacher_id = ? AND deleted_at IS NULL", teacherID).
		Scan(&classIDs).Error; err != nil {
		return fmt.Errorf("seed: look up classes for sessions: %w", err)
	}
	if len(classIDs) == 0 {
		log.Info("seed: no classes to generate sessions for, skipping", "teacher_id", teacherID)
		return nil
	}

	txMgr := database.NewTxManager(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db))
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	attendanceSvc := attendance.NewService(attendance.NewRepository(db), enrollmentsSvc, sessionsSvc, txMgr)

	// "Today" is the teacher's calendar day, matching how the app decides
	// which sessions count as already held. Session dates are stored as
	// UTC-midnight date values, so the comparison boundary is built in UTC
	// from the teacher-local date components.
	loc, err := time.LoadLocation(defaultTimezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	todayMid := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	to := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1)

	var generated int
	var past []pastSession
	// Every generated date per class, so the current-month backfill below can
	// avoid the unique (class_id, session_date) constraint.
	classDates := make(map[uuid.UUID]map[string]bool, len(classIDs))
	for _, classID := range classIDs {
		// Seeds have not been re-keyed to center scope yet; this shim carries
		// only the teacher id, so sessions' scoped query still resolves
		// tenancy by teacher until seeds gets its own sweep.
		rows, err := sessionsSvc.ListRange(ctx, authctx.Scope{TeacherID: teacherID}, classID, from, to)
		if err != nil {
			return fmt.Errorf("seed: generate sessions for class %s: %w", classID, err)
		}
		generated += len(rows)
		dates := make(map[string]bool, len(rows))
		for _, r := range rows {
			dates[r.SessionDate.Format("2006-01-02")] = true
			// Only sessions strictly before today have actually been held; a
			// session scheduled for today hasn't happened yet and must not be
			// seeded as pending attendance — the dashboard would (correctly)
			// not count it, leaving the seed's promised pending items short.
			if !r.SessionDate.Before(todayMid) {
				continue
			}
			past = append(past, pastSession{ID: r.ID, ClassID: classID, Date: r.SessionDate})
		}
		classDates[classID] = dates
	}
	sort.Slice(past, func(i, j int) bool { return past[i].Date.Before(past[j].Date) })

	confirmUpTo := max(len(past)-pendingAttendanceCount, 0)
	var confirmed int
	for i, ps := range past[:confirmUpTo] {
		// Seeds have not been re-keyed to center scope yet; this shim carries
		// only the teacher id, so enrollments' scoped query still resolves
		// tenancy by teacher until seeds gets its own sweep.
		roster, err := enrollmentsSvc.ActiveOn(ctx, authctx.Scope{TeacherID: teacherID}, ps.ClassID, ps.Date)
		if err != nil {
			return fmt.Errorf("seed: look up roster for session %s: %w", ps.ID, err)
		}
		var absentIDs []uuid.UUID
		for j, e := range roster {
			// A deterministic, evenly scattered "about one in eleven"
			// absence pattern — enough variation for demo data without
			// depending on a seeded random source.
			if (i*7+j*3)%11 == 0 {
				absentIDs = append(absentIDs, e.StudentID)
			}
		}
		req := attendance.ConfirmRequest{AbsentStudentIDs: absentIDs}
		if _, err := attendanceSvc.Confirm(ctx, teacherID, ps.ID, req); err != nil {
			return fmt.Errorf("seed: confirm attendance for session %s: %w", ps.ID, err)
		}
		confirmed++
	}

	// Early in a month, a class whose weekday hasn't come around yet has no
	// billable session in the current billing period, so billing previews and
	// parent statements for its families would be empty until the first
	// scheduled date passes. Backfill one ad-hoc, fully-confirmed session (a
	// make-up class, in product terms) for each such class so every seeded
	// family is billable in the current period on any calendar day. Billing
	// only counts confirmed sessions, and the most recent past sessions were
	// deliberately left pending above — so the check must look at what was
	// actually confirmed, not merely at what has passed. The backfilled
	// session is confirmed immediately and never joins the pending selection.
	var backfilled int
	confirmedPast := past[:confirmUpTo]
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, classID := range classIDs {
		hasCurrentMonthConfirmed := false
		for _, ps := range confirmedPast {
			if ps.ClassID == classID && !ps.Date.Before(monthStart) {
				hasCurrentMonthConfirmed = true
				break
			}
		}
		if hasCurrentMonthConfirmed {
			continue
		}
		candidate := time.Time{}
		for d := todayMid.AddDate(0, 0, -1); !d.Before(monthStart); d = d.AddDate(0, 0, -1) {
			if !classDates[classID][d.Format("2006-01-02")] {
				candidate = d
				break
			}
		}
		if candidate.IsZero() {
			// First day of the month: no strictly-past date exists in the
			// period, so fall back to today unless the schedule already
			// generated a session there.
			if classDates[classID][todayMid.Format("2006-01-02")] {
				log.Info("seed: no free date for current-month backfill, skipping", "class_id", classID)
				continue
			}
			candidate = todayMid
		}
		// Seeds have not been re-keyed to center scope yet; this shim carries
		// only the teacher id, so sessions' scoped query still resolves
		// tenancy by teacher until seeds gets its own sweep.
		detail, err := sessionsSvc.CreateAdHoc(ctx, authctx.Scope{TeacherID: teacherID}, classID, sessions.CreateSessionRequest{
			SessionDate: candidate.Format("2006-01-02"),
		})
		if err != nil {
			return fmt.Errorf("seed: backfill session for class %s: %w", classID, err)
		}
		if _, err := attendanceSvc.Confirm(ctx, teacherID, detail.ID, attendance.ConfirmRequest{}); err != nil {
			return fmt.Errorf("seed: confirm backfilled session %s: %w", detail.ID, err)
		}
		generated++
		confirmed++
		backfilled++
	}

	log.Info("seed: sessions generated", "teacher_id", teacherID, "sessions", generated,
		"attendance_confirmed", confirmed, "attendance_pending", len(past)-(confirmed-backfilled),
		"backfilled", backfilled)
	return nil
}
