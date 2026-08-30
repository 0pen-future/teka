//go:build integration

package imports_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/imports"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// Phones the workbooks below reference. They are written in the local 0-prefix
// form an operator types; the import normalises them to E.164 before matching
// the member directory.
const (
	namLocal = "0912345678"
	lanLocal = "0987654321"
)

// roster is the fixture center: an owner plus two teachers who share it.
type roster struct {
	svc         *imports.Service
	db          *gorm.DB
	owner       authctx.Scope
	nam, lan    uuid.UUID
	otherCenter authctx.Scope
	classesSvc  *classes.Service
	centersSvc  *centers.Service
}

func newRoster(t *testing.T) roster {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)

	centersSvc := centers.NewService(centers.NewRepository(db), txMgr, nil)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr, classstaff.NewRepository(db))
	contactsSvc := contacts.NewService(contacts.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db), nil)
	studentsSvc := students.NewService(students.NewRepository(db), enrollmentsSvc, txMgr)

	_, ownerTeacher := testutil.Teacher(t, db)
	owner := testutil.ScopeFor(t, db, ownerTeacher.ID)
	require.True(t, owner.IsOwner)

	_, nam := testutil.Teacher(t, db, testutil.WithPhone(namLocal))
	_, lan := testutil.Teacher(t, db, testutil.WithPhone(lanLocal))
	testutil.JoinCenter(t, db, nam.ID, owner.CenterID)
	testutil.JoinCenter(t, db, lan.ID, owner.CenterID)

	_, stranger := testutil.Teacher(t, db)

	return roster{
		svc: imports.NewService(centersSvc, classesSvc, contactsSvc, studentsSvc,
			enrollmentsSvc, imports.NewLocker(db), txMgr),
		db:          db,
		owner:       owner,
		nam:         nam.ID,
		lan:         lan.ID,
		otherCenter: testutil.ScopeFor(t, db, stranger.ID),
		classesSvc:  classesSvc,
		centersSvc:  centersSvc,
	}
}

func (r roster) do(t *testing.T, file []byte, dryRun bool) (*imports.Report, error) {
	t.Helper()
	return r.svc.Import(context.Background(), r.owner, file, dryRun)
}

// counts reads the five tables the import writes, scoped to the fixture
// center, so "wrote nothing" is checked against every one of them.
func (r roster) counts(t *testing.T) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, table := range []string{"classes", "class_schedules", "contacts", "students", "enrollments"} {
		var n int64
		require.NoError(t, r.db.Raw(
			"SELECT count(*) FROM "+table+" WHERE center_id = ? AND deleted_at IS NULL", r.owner.CenterID,
		).Scan(&n).Error)
		out[table] = n
	}
	return out
}

// workbook fills the shipped template with data rows, which also proves the
// template an operator downloads is one the parser accepts.
func workbook(t *testing.T, classRows, studentRows [][]string) []byte {
	t.Helper()
	tpl, err := imports.BuildTemplate()
	require.NoError(t, err)
	f, err := excelize.OpenReader(bytes.NewReader(tpl))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	// Row 1 is the header and row 2 the example, so data starts at row 3.
	write := func(sheet string, rows [][]string) {
		for i, row := range rows {
			for c, v := range row {
				cell, err := excelize.CoordinatesToCellName(c+1, i+3)
				require.NoError(t, err)
				require.NoError(t, f.SetCellStr(sheet, cell, v))
			}
		}
	}
	write(imports.SheetClasses, classRows)
	write(imports.SheetStudents, studentRows)

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf.Bytes()
}

// exampleRoster is the workbook from the template spec: two classes under two
// teachers, one running twice a week; three students; one parent with children
// under both teachers. Two of the three students have no distinguishing note,
// which is what makes the idempotency test meaningful.
func exampleRoster(t *testing.T) []byte {
	t.Helper()
	return workbook(t,
		[][]string{
			{"Toán 9A", namLocal, "01/09/2025", "150000", "2", "18:00", "90", ""},
			{"Toán 9A", namLocal, "01/09/2025", "150000", "5", "18:00", "90", ""},
			{"Văn 8", lanLocal, "15/09/2025", "120000", "CN", "08:30", "120", ""},
		},
		[][]string{
			{"Phạm Gia An", "Phạm Văn Hùng", "0901234567", "Toán 9A", namLocal, "", ""},
			{"Phạm Gia Bảo", "Phạm Văn Hùng", "0901234567", "Văn 8", lanLocal, "", ""},
			{"Lê Thu Hà", "Lê Thị Mai", "0977888999", "Toán 9A", namLocal, "05/10/2025", "Hà nhỏ"},
		})
}

func TestImportRoundTrip(t *testing.T) {
	t.Parallel()
	r := newRoster(t)

	rep, err := r.do(t, exampleRoster(t), false)
	require.NoError(t, err)
	require.True(t, rep.Committed)
	require.Equal(t, map[string]int64{
		"classes": 2, "class_schedules": 3, "contacts": 2, "students": 3, "enrollments": 3,
	}, r.counts(t))

	// Every row carries the class teacher's id, never the importing owner's.
	var anchors []struct {
		Name      string
		TeacherID uuid.UUID
	}
	require.NoError(t, r.db.Raw(
		"SELECT name, teacher_id FROM classes WHERE center_id = ? ORDER BY name", r.owner.CenterID,
	).Scan(&anchors).Error)
	require.Len(t, anchors, 2)
	require.Equal(t, "Toán 9A", anchors[0].Name)
	require.Equal(t, r.nam, anchors[0].TeacherID)
	require.Equal(t, "Văn 8", anchors[1].Name)
	require.Equal(t, r.lan, anchors[1].TeacherID)

	// One parent, children under two teachers, ONE contact row: contacts are
	// center data anchored on the owner, so a phone appears exactly once no
	// matter how many teachers its children study with.
	var hung []struct{ TeacherID uuid.UUID }
	require.NoError(t, r.db.Raw(
		"SELECT teacher_id FROM contacts WHERE center_id = ? AND phone = '+84901234567'",
		r.owner.CenterID,
	).Scan(&hung).Error)
	require.Len(t, hung, 1)
	require.Equal(t, r.owner.TeacherID, hung[0].TeacherID)

	// Students anchor on the owner too — only classes (and their enrollments)
	// keep the workbook teacher as pedagogical anchor.
	var studentAnchors []struct{ TeacherID uuid.UUID }
	require.NoError(t, r.db.Raw(
		"SELECT DISTINCT teacher_id FROM students WHERE center_id = ?", r.owner.CenterID,
	).Scan(&studentAnchors).Error)
	require.Len(t, studentAnchors, 1)
	require.Equal(t, r.owner.TeacherID, studentAnchors[0].TeacherID)

	// The enrollment inherits the class's unit price; the import never sets one.
	var prices []int64
	require.NoError(t, r.db.Raw(`
		SELECT e.unit_price FROM enrollments e
		JOIN classes c ON c.id = e.class_id
		WHERE e.center_id = ? AND c.name = 'Toán 9A'`, r.owner.CenterID).Scan(&prices).Error)
	require.Equal(t, []int64{150000, 150000}, prices)

	// Imported classes go through the same create hook as the API: each class
	// is born with exactly one active giao_vien stint for its anchor teacher.
	var stints []struct {
		Name      string
		TeacherID uuid.UUID
	}
	require.NoError(t, r.db.Raw(`
		SELECT c.name, cs.teacher_id FROM class_staff cs
		JOIN classes c ON c.id = cs.class_id
		WHERE cs.center_id = ? AND cs.role_key = 'giao_vien' AND cs.ended_at IS NULL
		ORDER BY c.name`, r.owner.CenterID).Scan(&stints).Error)
	require.Len(t, stints, 2)
	require.Equal(t, r.nam, stints[0].TeacherID)
	require.Equal(t, r.lan, stints[1].TeacherID)

	// A blank Ngày nhập học falls back to the class start date, not today.
	var started time.Time
	require.NoError(t, r.db.Raw(`
		SELECT e.started_on FROM enrollments e
		JOIN students s ON s.id = e.student_id
		WHERE e.center_id = ? AND s.full_name = 'Phạm Gia An'`, r.owner.CenterID).Scan(&started).Error)
	require.Equal(t, "2025-09-01", started.Format("2006-01-02"))
}

func TestReimportOfTheSameFileWritesNothing(t *testing.T) {
	t.Parallel()
	r := newRoster(t)
	file := exampleRoster(t)

	_, err := r.do(t, file, false)
	require.NoError(t, err)
	before := r.counts(t)
	stamps := r.touchStamps(t)

	// Two of the three students carry no note, stored as NULL. A lookup using
	// `display_note = ''` would miss them and duplicate the roster, which is
	// invisible in a fixture where every student has a note.
	rep, err := r.do(t, file, false)
	require.NoError(t, err)

	require.Equal(t, 0, rep.Classes.Created)
	require.Equal(t, 0, rep.Schedules.Created)
	require.Equal(t, 0, rep.Contacts.Created)
	require.Equal(t, 0, rep.Students.Created)
	require.Equal(t, 0, rep.Enrollments.Created)
	require.Equal(t, 2, rep.Classes.Reused)
	require.Equal(t, 3, rep.Students.Reused)

	require.Equal(t, before, r.counts(t))
	require.Equal(t, stamps, r.touchStamps(t), "a re-import must not even touch updated_at")
}

// touchStamps collects updated_at for every row the import can reach.
func (r roster) touchStamps(t *testing.T) []time.Time {
	t.Helper()
	var out []time.Time
	for _, table := range []string{"classes", "class_schedules", "contacts", "students", "enrollments"} {
		var stamps []struct{ UpdatedAt time.Time }
		require.NoError(t, r.db.Raw(
			"SELECT updated_at FROM "+table+" WHERE center_id = ? ORDER BY id", r.owner.CenterID,
		).Scan(&stamps).Error)
		for _, s := range stamps {
			out = append(out, s.UpdatedAt)
		}
	}
	return out
}

func TestImportingThreeTimesLeavesThreeSchedules(t *testing.T) {
	t.Parallel()
	r := newRoster(t)
	file := exampleRoster(t)

	for range 3 {
		_, err := r.do(t, file, false)
		require.NoError(t, err)
	}

	// class_schedules has no unique index, so nothing but the effective_from
	// pre-check stops a re-import from appending another identical slot.
	require.Equal(t, int64(3), r.counts(t)["class_schedules"])
}

func TestAnInvalidRowLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	r := newRoster(t)

	// The last student row names a class that is not in the file, so the file
	// is rejected after the first two rows were otherwise importable.
	file := workbook(t,
		[][]string{{"Toán 9A", namLocal, "01/09/2025", "150000", "2", "18:00", "90", ""}},
		[][]string{
			{"Phạm Gia An", "Phạm Văn Hùng", "0901234567", "Toán 9A", namLocal, "", ""},
			{"Lê Thu Hà", "Lê Thị Mai", "0977888999", "Lớp không có", namLocal, "", ""},
		})

	_, err := r.do(t, file, false)
	require.Equal(t, []string{imports.CodeClassNotInFile}, rowErrorCodes(t, err))
	require.Equal(t, map[string]int64{
		"classes": 0, "class_schedules": 0, "contacts": 0, "students": 0, "enrollments": 0,
	}, r.counts(t), "a partial roster is worse than none")
}

func TestClassNameOverflowIsARowErrorNotADatabaseError(t *testing.T) {
	t.Parallel()
	r := newRoster(t)

	file := workbook(t,
		[][]string{{strings.Repeat("A", imports.MaxClassName+1), namLocal, "01/09/2025", "150000", "2", "18:00", "90", ""}},
		nil)

	// classes.name is VARCHAR(100): without the length check this would reach
	// Postgres as a 22001 and surface as a 500.
	_, err := r.do(t, file, true)
	require.Equal(t, []string{imports.CodeTooLong}, rowErrorCodes(t, err))
}

func TestClassWithNoTeacherIsAnchoredOnTheOwner(t *testing.T) {
	t.Parallel()
	r := newRoster(t)

	file := workbook(t,
		[][]string{{"Lớp chưa phân công", "", "01/09/2025", "150000", "2", "18:00", "90", ""}},
		[][]string{{"Phạm Gia An", "Phạm Văn Hùng", "0901234567", "Lớp chưa phân công", "", "", ""}})

	_, err := r.do(t, file, false)
	require.NoError(t, err)

	// A bare uuid.UUID destination skips its sql.Scanner, so the id lands in
	// a struct field — the same shape testutil.ScopeFor scans into.
	var row struct{ TeacherID uuid.UUID }
	require.NoError(t, r.db.Raw(
		"SELECT teacher_id FROM classes WHERE center_id = ? AND name = 'Lớp chưa phân công'", r.owner.CenterID,
	).Scan(&row).Error)
	require.Equal(t, r.owner.TeacherID, row.TeacherID,
		"a class with nobody assigned belongs to the owner, who is a teacher too")
}

// TestGrantedMemberImportAnchorsEverythingOnTheOwner runs the import as a
// member holding the imports.run grant: the run succeeds, yet every contact
// and student row still anchors on the center OWNER — the anchor comes from
// ownership resolution, never from the caller. A member without the grant is
// refused outright, and an owner re-import of the same file finds everything
// already in place.
func TestGrantedMemberImportAnchorsEverythingOnTheOwner(t *testing.T) {
	t.Parallel()
	r := newRoster(t)
	ctx := context.Background()

	// nam holds imports.run through a member grant, resolved by the real scope
	// pipeline — the same rows and query the middleware uses.
	require.NoError(t, r.db.Exec(
		"INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed) VALUES (?, ?, ?, TRUE)",
		r.nam, r.owner.CenterID, authctx.PermImportsRun).Error)
	scNam, err := r.centersSvc.ResolveScope(ctx, r.nam)
	require.NoError(t, err)
	require.False(t, scNam.IsOwner)
	require.True(t, scNam.Has(authctx.PermImportsRun))
	scLan, err := r.centersSvc.ResolveScope(ctx, r.lan)
	require.NoError(t, err)

	_, err = r.svc.Import(ctx, scLan, exampleRoster(t), false)
	require.Equal(t, 403, apperror.From(err).Status, "no grant, no import")
	require.Equal(t, int64(0), r.counts(t)["classes"])

	rep, err := r.svc.Import(ctx, scNam, exampleRoster(t), false)
	require.NoError(t, err)
	require.True(t, rep.Committed)

	var anchors []struct{ TeacherID uuid.UUID }
	require.NoError(t, r.db.Raw(`
		SELECT teacher_id FROM contacts WHERE center_id = ?
		UNION SELECT teacher_id FROM students WHERE center_id = ?`,
		r.owner.CenterID, r.owner.CenterID).Scan(&anchors).Error)
	require.Len(t, anchors, 1, "one anchor for every contact and student row")
	require.Equal(t, r.owner.TeacherID, anchors[0].TeacherID)

	// The owner re-importing the member's file must reuse, not duplicate —
	// both runs resolve the same owner anchor.
	before := r.counts(t)
	rep, err = r.do(t, exampleRoster(t), false)
	require.NoError(t, err)
	require.Equal(t, 0, rep.Contacts.Created)
	require.Equal(t, 0, rep.Students.Created)
	require.Equal(t, before, r.counts(t))
}

func TestAPhoneOutsideTheCenterIsRefusedBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	r := newRoster(t)

	// Same shape as a typo that lands on a real teacher of another center.
	_, other := testutil.Teacher(t, r.db, testutil.WithPhone("0933222111"))
	require.NotEqual(t, r.owner.CenterID, testutil.ScopeFor(t, r.db, other.ID).CenterID)

	file := workbook(t,
		[][]string{{"Toán 9A", "0933222111", "01/09/2025", "150000", "2", "18:00", "90", ""}},
		nil)

	_, err := r.do(t, file, false)
	require.Equal(t, []string{imports.CodeTeacherNotInCenter}, rowErrorCodes(t, err))
	require.Equal(t, int64(0), r.counts(t)["classes"])
}

func TestARemovedMemberIsRefusedByResolutionNotByTheForeignKey(t *testing.T) {
	t.Parallel()
	r := newRoster(t)

	// Offboarding closes the stint and disables the account (RemoveMember,
	// centers/service.go). The center_members row survives with left_at set,
	// and the FK guard targets the table's PRIMARY KEY, so it still holds —
	// only ListMembers' ua.status = active join catches this.
	require.NoError(t, r.db.Exec(
		"UPDATE center_members SET left_at = now() WHERE teacher_id = ? AND center_id = ?",
		r.nam, r.owner.CenterID).Error)
	require.NoError(t, r.db.Exec(
		"UPDATE user_accounts SET status = 'disabled' WHERE id = ?", r.nam).Error)

	file := workbook(t,
		[][]string{{"Toán 9A", namLocal, "01/09/2025", "150000", "2", "18:00", "90", ""}},
		nil)

	_, err := r.do(t, file, false)
	require.Equal(t, []string{imports.CodeTeacherNotInCenter}, rowErrorCodes(t, err))

	// And the FK really does not object, which is why the check above matters.
	var stillReferenced int64
	require.NoError(t, r.db.Raw(
		"SELECT count(*) FROM center_members WHERE teacher_id = ? AND center_id = ?",
		r.nam, r.owner.CenterID).Scan(&stillReferenced).Error)
	require.Equal(t, int64(1), stillReferenced)
}

func TestAnAnchorFromAnotherCenterIsRefusedByTheForeignKey(t *testing.T) {
	t.Parallel()
	r := newRoster(t)

	// The import can never build this scope — the directory it resolves
	// phones through is keyed on the caller's own center. The composite FK is
	// the backstop if that ever regresses.
	crossCenter := authctx.Scope{TeacherID: r.otherCenter.TeacherID, CenterID: r.owner.CenterID}
	price := int64(150000)
	_, err := r.classesSvc.Create(context.Background(), crossCenter, classes.CreateClassRequest{
		Name:             "Lớp lạc trung tâm",
		StartDate:        "2025-09-01",
		DefaultUnitPrice: &price,
		Schedules:        []classes.ScheduleRequest{{Weekday: weekdayPtr(1), StartTime: "18:00", DurationMin: 90}},
	})
	require.Error(t, err)
	require.Equal(t, int64(0), r.counts(t)["classes"])
}

func weekdayPtr(v int16) *int16 { return &v }

// rowErrorCodes pulls the codes out of the 422 the import returns for a file
// with defective rows.
func rowErrorCodes(t *testing.T, err error) []string {
	t.Helper()
	var rowErrs *imports.RowErrorsError
	require.ErrorAs(t, err, &rowErrs)
	codes := make([]string, 0, len(rowErrs.Payload.Errors))
	for _, e := range rowErrs.Payload.Errors {
		codes = append(codes, e.Code)
	}
	return codes
}
