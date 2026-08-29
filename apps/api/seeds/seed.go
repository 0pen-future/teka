// Package seeds populates development data. Runs are idempotent: teachers are
// keyed by phone, roster data by the owning teacher having none yet, and
// existing rows are never modified, so reseeding a database with real data is
// safe.
package seeds

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/billing"
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
// The first teacher owns the center; every following teacher joins that same
// center as a member instead of owning a center of their own — the invite-only
// onboarding funnel has no self-registration or join-by-phone path anymore, so
// a stable owner+member pair in one center is what the web and e2e specs need
// to log in as.
var seedTeachers = []seedTeacher{
	{Phone: "+84901000001", Password: "lan-password", FullName: "Cô Lan"},
	{Phone: "+84901000002", Password: "minh-password", FullName: "Thầy Minh"},
}

// The secretary is a member with no teaching data of her own: delegated
// sending (can_send_reports) is only observable on someone whose center-wide
// reach comes entirely from the flag. The seed never sets the flag — the
// grant happens through the owner's UI, so the database stays neutral for
// specs that expect a plain member.
var seedSecretary = seedTeacher{Phone: "+84901000003", Password: "thu-password", FullName: "Cô Thu"}

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

// teacherDataset groups one teaching member's demo roster so Run can seed
// several teachers through the same functions.
type teacherDataset struct {
	Contacts    []seedContact
	Students    []seedStudent
	Classes     []seedClass
	Enrollments []seedEnrollment
}

var ownerData = teacherDataset{
	Contacts:    seedContacts,
	Students:    seedStudents,
	Classes:     seedClasses,
	Enrollments: seedEnrollments,
}

// Thầy Minh teaches a small class of his own so the center holds
// cross-teacher data: center-wide oversight and delegated sends are only
// observable when a billable period belongs to someone other than the owner.
var minhData = teacherDataset{
	Contacts: []seedContact{
		{Phone: "+84913000001", FullName: "Chị Yến"},
		{Phone: "+84913000002", FullName: "Anh Sơn"},
	},
	Students: []seedStudent{
		{FullName: "Bé Phúc", ContactPhone: "+84913000001"},
		{FullName: "Bé Quỳnh", ContactPhone: "+84913000002"},
	},
	Classes: []seedClass{
		{
			Name:      "Lý 7 - Chiều Thứ Năm",
			StartDate: "2026-02-05",
			UnitPrice: 180_000,
			Schedules: []seedSchedule{
				{Weekday: 4, StartTime: "17:00", DurationMin: 90},
			},
		},
	},
	Enrollments: []seedEnrollment{
		{StudentName: "Bé Phúc", ClassName: "Lý 7 - Chiều Thứ Năm", StartedOn: "2026-02-05"},
		{StudentName: "Bé Quỳnh", ClassName: "Lý 7 - Chiều Thứ Năm", StartedOn: "2026-02-05"},
	},
}

// Run inserts the seed teachers that do not exist yet — the first as the
// center's owner, every following one as a member of that same center — then
// demo roster data for each teaching member, an open billing period for the
// member teacher, and reports each outcome.
func Run(ctx context.Context, db *gorm.DB, log *slog.Logger) error {
	ownerID, centerID, err := ensureOwner(ctx, db, log, seedTeachers[0])
	if err != nil {
		return err
	}
	minhID, err := ensureMember(ctx, db, log, seedTeachers[1], centerID)
	if err != nil {
		return err
	}
	if _, err := ensureMember(ctx, db, log, seedSecretary, centerID); err != nil {
		return err
	}

	ownerSc, err := scopeFor(ctx, db, ownerID)
	if err != nil {
		return err
	}
	minhSc, err := scopeFor(ctx, db, minhID)
	if err != nil {
		return err
	}

	for _, t := range []struct {
		sc      authctx.Scope
		data    teacherDataset
		pending int
	}{
		// The owner keeps pending sessions so the dashboard's attendance
		// warning has material; the member's sessions are all confirmed so his
		// current period can close below — statements (and therefore report
		// sends) only exist for a closed period.
		{ownerSc, ownerData, pendingAttendanceCount},
		{minhSc, minhData, 0},
	} {
		if err := seedRoster(ctx, db, log, t.sc, t.data.Contacts); err != nil {
			return err
		}
		if err := seedStudentList(ctx, db, log, t.sc, t.data.Students); err != nil {
			return err
		}
		if err := seedClassList(ctx, db, log, t.sc, t.data.Classes); err != nil {
			return err
		}
		if err := seedEnrollmentList(ctx, db, log, t.sc, t.data.Enrollments); err != nil {
			return err
		}
		if err := seedSessionList(ctx, db, log, t.sc, t.pending); err != nil {
			return err
		}
	}

	// Only the member teacher gets a pre-closed period: a delegated sender can
	// read and send another teacher's period but never create or close it
	// (EnsurePeriod self-assigns to the caller, close is a write), and sends
	// only work on a closed period. The owner keeps opening and closing her
	// own period through the UI.
	return seedClosedPeriod(ctx, db, log, minhSc)
}

// scopeFor resolves the seeded teacher's live center scope the same way the
// scope middleware does, so the service calls below run with the exact tenant
// context a real request would carry.
func scopeFor(ctx context.Context, db *gorm.DB, teacherID uuid.UUID) (authctx.Scope, error) {
	// Scanning straight into a bare uuid.UUID skips its sql.Scanner and hits
	// GORM's element-wise array path instead; wrap it in a struct field.
	var row struct {
		CenterID       uuid.UUID
		IsOwner        bool
		CanSendReports bool
	}
	err := db.WithContext(ctx).Raw(`
		SELECT t.center_id, (c.owner_id = t.id) AS is_owner,
			COALESCE(cm.can_send_reports, FALSE) AS can_send_reports
		FROM teachers t
		JOIN centers c ON c.id = t.center_id
		LEFT JOIN center_members cm ON cm.teacher_id = t.id
			AND cm.center_id = t.center_id AND cm.left_at IS NULL
		WHERE t.id = ?`, teacherID).Scan(&row).Error
	if err != nil {
		return authctx.Scope{}, fmt.Errorf("seed: resolve scope for %s: %w", teacherID, err)
	}
	if row.CenterID == uuid.Nil {
		return authctx.Scope{}, fmt.Errorf("seed: teacher %s has no center", teacherID)
	}
	return authctx.Scope{
		TeacherID:      teacherID,
		CenterID:       row.CenterID,
		IsOwner:        row.IsOwner,
		CanSendReports: row.CanSendReports,
	}, nil
}

// accountExists looks up an account by phone, so both ensureOwner and
// ensureMember can share the same idempotency check.
func accountExists(ctx context.Context, db *gorm.DB, phone string) (uuid.UUID, bool, error) {
	var existing []uuid.UUID
	err := db.WithContext(ctx).
		Raw("SELECT id FROM user_accounts WHERE phone = ? AND deleted_at IS NULL", phone).
		Scan(&existing).Error
	if err != nil {
		return uuid.UUID{}, false, fmt.Errorf("seed: look up %s: %w", phone, err)
	}
	if len(existing) > 0 {
		return existing[0], true, nil
	}
	return uuid.UUID{}, false, nil
}

// ensureOwner returns the id of the teacher with s.Phone and the id of the
// center they own, creating both the center and the user_accounts + teachers
// row pair in one transaction when the account is absent.
func ensureOwner(ctx context.Context, db *gorm.DB, log *slog.Logger, s seedTeacher) (uuid.UUID, uuid.UUID, error) {
	if existingID, ok, err := accountExists(ctx, db, s.Phone); err != nil {
		return uuid.UUID{}, uuid.UUID{}, err
	} else if ok {
		log.Info("seed: teacher exists, skipping", "phone", s.Phone)
		// Scanning straight into a bare uuid.UUID skips its sql.Scanner and hits
		// GORM's element-wise array path instead; wrap it in a struct field, same
		// as scopeFor below.
		var row struct{ CenterID uuid.UUID }
		if err := db.WithContext(ctx).
			Raw("SELECT center_id FROM teachers WHERE id = ?", existingID).
			Scan(&row).Error; err != nil {
			return uuid.UUID{}, uuid.UUID{}, fmt.Errorf("seed: resolve center for %s: %w", s.Phone, err)
		}
		return existingID, row.CenterID, nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(s.Password), bcryptCost)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, fmt.Errorf("seed: hash password for %s: %w", s.Phone, err)
	}
	accountID := id.New()
	centerID := id.New()
	// One transaction, mirroring registration: the personal centers row first
	// (its owner FK is deferred until the teachers row follows), then the
	// account/teacher pair, then the live membership stint.
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"INSERT INTO centers (id, name, owner_id) VALUES (?, ?, ?)",
			centerID, s.FullName, accountID,
		).Error; err != nil {
			return err
		}
		// Every center carries its three system roles from birth, the same
		// invariant repository CreateCenter enforces; the owner's own
		// membership stays roleless (owner is outside the role system).
		if err := tx.Exec(`
			INSERT INTO center_roles (id, center_id, key, name)
			VALUES (gen_random_uuid(), @cid, 'giao_vien', 'Giáo viên'),
				(gen_random_uuid(), @cid, 'hoc_vu', 'Học vụ'),
				(gen_random_uuid(), @cid, 'tro_giang', 'Trợ giảng')`,
			map[string]any{"cid": centerID},
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			"INSERT INTO user_accounts (id, role, phone, password_hash, status) VALUES (?, 'teachers', ?, ?, 'active')",
			accountID, s.Phone, string(hash),
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			"INSERT INTO teachers (id, full_name, timezone, center_id) VALUES (?, ?, ?, ?)",
			accountID, s.FullName, defaultTimezone, centerID,
		).Error; err != nil {
			return err
		}
		return tx.Exec(
			"INSERT INTO center_members (teacher_id, center_id) VALUES (?, ?)",
			accountID, centerID,
		).Error
	})
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, fmt.Errorf("seed: create %s: %w", s.Phone, err)
	}
	log.Info("seed: owner created", "phone", s.Phone, "full_name", s.FullName)
	return accountID, centerID, nil
}

// ensureMember returns the id of the teacher with s.Phone, creating the
// user_accounts + teachers row pair as a member of centerID (no center of
// their own) when absent — mirroring what invitation-accept produces, without
// the token round-trip.
func ensureMember(ctx context.Context, db *gorm.DB, log *slog.Logger, s seedTeacher, centerID uuid.UUID) (uuid.UUID, error) {
	if existingID, ok, err := accountExists(ctx, db, s.Phone); err != nil {
		return uuid.UUID{}, err
	} else if ok {
		log.Info("seed: teacher exists, skipping", "phone", s.Phone)
		return existingID, nil
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
		if err := tx.Exec(
			"INSERT INTO teachers (id, full_name, timezone, center_id) VALUES (?, ?, ?, ?)",
			accountID, s.FullName, defaultTimezone, centerID,
		).Error; err != nil {
			return err
		}
		return tx.Exec(
			"INSERT INTO center_members (teacher_id, center_id) VALUES (?, ?)",
			accountID, centerID,
		).Error
	})
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("seed: create %s: %w", s.Phone, err)
	}
	log.Info("seed: member created", "phone", s.Phone, "full_name", s.FullName, "center_id", centerID)
	return accountID, nil
}

// seedRoster inserts demo contacts for one teacher. It is all-or-nothing per
// teacher: any pre-existing contact means the roster was seeded (or the
// teacher has real data) and the whole block is skipped.
func seedRoster(ctx context.Context, db *gorm.DB, log *slog.Logger, sc authctx.Scope, contacts []seedContact) error {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT count(*) FROM contacts WHERE teacher_id = ? AND deleted_at IS NULL", sc.TeacherID).
		Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up contacts: %w", err)
	}
	if count > 0 {
		log.Info("seed: roster exists, skipping", "teacher_id", sc.TeacherID)
		return nil
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, c := range contacts {
			if err := tx.Exec(
				"INSERT INTO contacts (id, teacher_id, center_id, full_name, phone) VALUES (?, ?, ?, ?, ?)",
				id.New(), sc.TeacherID, sc.CenterID, c.FullName, c.Phone,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("seed: create roster: %w", err)
	}
	log.Info("seed: roster created", "teacher_id", sc.TeacherID, "contacts", len(contacts))
	return nil
}

// seedStudentList inserts the demo students against the seeded contacts,
// skipped wholesale when the teacher already has any student. Contacts are
// resolved by phone so the block also works against a roster seeded earlier.
func seedStudentList(ctx context.Context, db *gorm.DB, log *slog.Logger, sc authctx.Scope, students []seedStudent) error {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT count(*) FROM students WHERE teacher_id = ? AND deleted_at IS NULL", sc.TeacherID).
		Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up students: %w", err)
	}
	if count > 0 {
		log.Info("seed: students exist, skipping", "teacher_id", sc.TeacherID)
		return nil
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, s := range students {
			var contactIDs []uuid.UUID
			if err := tx.Raw(
				"SELECT id FROM contacts WHERE teacher_id = ? AND phone = ? AND deleted_at IS NULL",
				sc.TeacherID, s.ContactPhone,
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
				"INSERT INTO students (id, teacher_id, center_id, contact_id, full_name, display_note) VALUES (?, ?, ?, ?, ?, ?)",
				id.New(), sc.TeacherID, sc.CenterID, contactIDs[0], s.FullName, note,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("seed: create students: %w", err)
	}
	log.Info("seed: students created", "teacher_id", sc.TeacherID, "students", len(students))
	return nil
}

// seedClassList inserts the demo classes with their weekly schedules for one
// teacher, skipped wholesale when the teacher already has any class.
func seedClassList(ctx context.Context, db *gorm.DB, log *slog.Logger, sc authctx.Scope, classList []seedClass) error {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT count(*) FROM classes WHERE teacher_id = ? AND deleted_at IS NULL", sc.TeacherID).
		Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up classes: %w", err)
	}
	if count > 0 {
		log.Info("seed: classes exist, skipping", "teacher_id", sc.TeacherID)
		return nil
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, c := range classList {
			classID := id.New()
			if err := tx.Exec(
				"INSERT INTO classes (id, teacher_id, center_id, name, start_date, default_unit_price, status) VALUES (?, ?, ?, ?, ?::date, ?, 'active')",
				classID, sc.TeacherID, sc.CenterID, c.Name, c.StartDate, c.UnitPrice,
			).Error; err != nil {
				return err
			}
			for _, s := range c.Schedules {
				if err := tx.Exec(
					"INSERT INTO class_schedules (id, teacher_id, center_id, class_id, weekday, start_time, duration_min, effective_from) VALUES (?, ?, ?, ?, ?, ?::time, ?, ?::date)",
					id.New(), sc.TeacherID, sc.CenterID, classID, s.Weekday, s.StartTime, s.DurationMin, c.StartDate,
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
	log.Info("seed: classes created", "teacher_id", sc.TeacherID, "classes", len(classList))
	return nil
}

// seedEnrollmentList enrolls the seeded students into the seeded classes,
// skipped wholesale when the teacher already has any enrollment. Students and
// classes are resolved by name; unit_price is copied from the class's current
// default, matching what the enrollments service does.
func seedEnrollmentList(ctx context.Context, db *gorm.DB, log *slog.Logger, sc authctx.Scope, enrollmentList []seedEnrollment) error {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT count(*) FROM enrollments WHERE teacher_id = ? AND deleted_at IS NULL", sc.TeacherID).
		Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up enrollments: %w", err)
	}
	if count > 0 {
		log.Info("seed: enrollments exist, skipping", "teacher_id", sc.TeacherID)
		return nil
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, e := range enrollmentList {
			var studentIDs []uuid.UUID
			if err := tx.Raw(
				"SELECT id FROM students WHERE teacher_id = ? AND full_name = ? AND deleted_at IS NULL",
				sc.TeacherID, e.StudentName,
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
				sc.TeacherID, e.ClassName,
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
				"INSERT INTO enrollments (id, teacher_id, center_id, student_id, class_id, started_on, ended_on, unit_price) VALUES (?, ?, ?, ?, ?, ?::date, ?::date, ?)",
				id.New(), sc.TeacherID, sc.CenterID, studentIDs[0], classRows[0].ID, e.StartedOn, endedOn, classRows[0].DefaultUnitPrice,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("seed: create enrollments: %w", err)
	}
	log.Info("seed: enrollments created", "teacher_id", sc.TeacherID, "enrollments", len(enrollmentList))
	return nil
}

// seedServices wires the real feature services the seed drives, mirroring
// router.go's construction so seeded data flows through production code paths.
type seedServices struct {
	enrollments *enrollments.Service
	sessions    *sessions.Service
	attendance  *attendance.Service
	billing     *billing.Service
}

func newSeedServices(db *gorm.DB) seedServices {
	txMgr := database.NewTxManager(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db))
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	attendanceSvc := attendance.NewService(attendance.NewRepository(db), enrollmentsSvc, sessionsSvc, txMgr)
	billingSvc := billing.NewService(billing.NewRepository(db, attendanceSvc), txMgr, sessionsSvc, enrollmentsSvc)
	return seedServices{
		enrollments: enrollmentsSvc,
		sessions:    sessionsSvc,
		attendance:  attendanceSvc,
		billing:     billingSvc,
	}
}

// seedClosedPeriod opens the current calendar month's billing period for one
// teacher through the real billing service, then closes it — statements, and
// with them report sends, only exist for a closed period. EnsurePeriod
// converges on the existing (teacher, year, month) row and an already-closed
// period skips the close, so reseeding a reused database changes nothing.
// Close refuses unconfirmed past sessions, so this teacher's seed must
// confirm every past session (pending count 0).
func seedClosedPeriod(ctx context.Context, db *gorm.DB, log *slog.Logger, sc authctx.Scope) error {
	loc, err := time.LoadLocation(defaultTimezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	svcs := newSeedServices(db)
	period, err := svcs.billing.EnsurePeriod(ctx, sc, now.Year(), int(now.Month()))
	if err != nil {
		return fmt.Errorf("seed: ensure period for %s: %w", sc.TeacherID, err)
	}
	if period.Status != billing.PeriodOpen {
		log.Info("seed: billing period already closed, skipping close",
			"teacher_id", sc.TeacherID, "period_id", period.ID)
		return nil
	}
	closed, err := svcs.billing.Close(ctx, sc, period.ID)
	var unconfirmed *billing.ErrUnconfirmedSessions
	if errors.As(err, &unconfirmed) {
		// A reused database can hold sessions this run did not create (an
		// older seed shape, or manual dev work on the demo teacher). Closing
		// is irreversible, so leave the period open rather than failing the
		// whole seed; sends just stay unavailable until it is closed by hand.
		log.Warn("seed: period has unconfirmed sessions, leaving it open",
			"teacher_id", sc.TeacherID, "period_id", period.ID,
			"unconfirmed", len(unconfirmed.Sessions))
		return nil
	}
	if err != nil {
		return fmt.Errorf("seed: close period %s: %w", period.ID, err)
	}
	log.Info("seed: billing period closed", "teacher_id", sc.TeacherID,
		"period_id", period.ID, "invoices_issued", closed.IssuedCount)
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
// real attendance.Service, for every past session except the pendingCount
// most recent — a deterministic scatter of absences gives the seeded data
// realistic variation without needing true randomness. pendingCount 0 leaves
// nothing unconfirmed, which is what lets a teacher's period close.
func seedSessionList(ctx context.Context, db *gorm.DB, log *slog.Logger, sc authctx.Scope, pendingCount int) error {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT count(*) FROM class_sessions WHERE teacher_id = ? AND deleted_at IS NULL", sc.TeacherID).
		Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up sessions: %w", err)
	}
	if count > 0 {
		log.Info("seed: sessions exist, skipping", "teacher_id", sc.TeacherID)
		return nil
	}

	var classIDs []uuid.UUID
	if err := db.WithContext(ctx).
		Raw("SELECT id FROM classes WHERE teacher_id = ? AND deleted_at IS NULL", sc.TeacherID).
		Scan(&classIDs).Error; err != nil {
		return fmt.Errorf("seed: look up classes for sessions: %w", err)
	}
	if len(classIDs) == 0 {
		log.Info("seed: no classes to generate sessions for, skipping", "teacher_id", sc.TeacherID)
		return nil
	}

	svcs := newSeedServices(db)
	enrollmentsSvc, sessionsSvc, attendanceSvc := svcs.enrollments, svcs.sessions, svcs.attendance

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
		rows, err := sessionsSvc.ListRange(ctx, sc, classID, from, to)
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

	confirmUpTo := max(len(past)-pendingCount, 0)
	var confirmed int
	for i, ps := range past[:confirmUpTo] {
		roster, err := enrollmentsSvc.ActiveOn(ctx, sc, ps.ClassID, ps.Date)
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
		if _, err := attendanceSvc.Confirm(ctx, sc, ps.ID, req); err != nil {
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
		detail, err := sessionsSvc.CreateAdHoc(ctx, sc, classID, sessions.CreateSessionRequest{
			SessionDate: candidate.Format("2006-01-02"),
		})
		if err != nil {
			return fmt.Errorf("seed: backfill session for class %s: %w", classID, err)
		}
		if _, err := attendanceSvc.Confirm(ctx, sc, detail.ID, attendance.ConfirmRequest{}); err != nil {
			return fmt.Errorf("seed: confirm backfilled session %s: %w", detail.ID, err)
		}
		generated++
		confirmed++
		backfilled++
	}

	log.Info("seed: sessions generated", "teacher_id", sc.TeacherID, "sessions", generated,
		"attendance_confirmed", confirmed, "attendance_pending", len(past)-(confirmed-backfilled),
		"backfilled", backfilled)
	return nil
}
