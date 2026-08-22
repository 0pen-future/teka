package imports

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var (
	namPhone = "+84912345678"
	lanPhone = "+84987654321"
)

func date(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(sheetDateLayout, s)
	require.NoError(t, err)
	return d
}

type fixture struct {
	nam, lan, owner uuid.UUID
	dir             map[string]uuid.UUID
}

func newFixture() fixture {
	nam, lan, owner := uuid.New(), uuid.New(), uuid.New()
	return fixture{
		nam: nam, lan: lan, owner: owner,
		dir: map[string]uuid.UUID{namPhone: nam, lanPhone: lan},
	}
}

func (f fixture) resolve(wb *ParsedWorkbook) (*resolvedPlan, []RowError) {
	return resolve(wb, f.dir, f.owner)
}

func exampleRoster(t *testing.T) *ParsedWorkbook {
	t.Helper()
	return &ParsedWorkbook{
		Classes: []ClassRow{
			{Line: 3, Name: "Toán 9A", TeacherPhone: namPhone, StartDate: date(t, "1/9/2025"), UnitPrice: 150000, Weekday: 1, StartTime: "18:00", DurationMin: 90},
			{Line: 4, Name: "Toán 9A", TeacherPhone: namPhone, StartDate: date(t, "1/9/2025"), UnitPrice: 150000, Weekday: 4, StartTime: "18:00", DurationMin: 90},
			{Line: 5, Name: "Văn 8", TeacherPhone: lanPhone, StartDate: date(t, "15/9/2025"), UnitPrice: 120000, Weekday: 0, StartTime: "08:30", DurationMin: 120},
		},
		Students: []StudentRow{
			{Line: 3, StudentName: "Phạm Gia An", ContactName: "Phạm Văn Hùng", ContactPhone: "+84901234567", ClassName: "Toán 9A", TeacherPhone: namPhone},
			{Line: 4, StudentName: "Phạm Gia Bảo", ContactName: "Phạm Văn Hùng", ContactPhone: "+84901234567", ClassName: "Văn 8", TeacherPhone: lanPhone},
			{Line: 5, StudentName: "Lê Thu Hà", ContactName: "Lê Thị Mai", ContactPhone: "+84977888999", ClassName: "Toán 9A", TeacherPhone: namPhone, DisplayNote: "Hà nhỏ"},
		},
	}
}

func TestResolveHappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture()
	plan, errs := f.resolve(exampleRoster(t))
	require.Empty(t, errs)

	require.Len(t, plan.classes, 2, "three slot rows collapse into two classes")
	require.Equal(t, f.nam, plan.classes[0].key.teacherID)
	require.Len(t, plan.classes[0].slots, 2, "Toán 9A runs twice a week")
	require.Len(t, plan.classes[1].slots, 1)

	require.Len(t, plan.students, 3)
	// A blank Ngày nhập học inherits the class start date, never today's.
	require.Equal(t, date(t, "1/9/2025"), plan.students[0].startedOn)
	require.Equal(t, date(t, "15/9/2025"), plan.students[1].startedOn)

	// One parent, two teachers, two contacts: uq_contacts_phone is
	// (teacher_id, phone), so this is two statement links and two balances.
	require.Equal(t, 3, distinctContacts(plan.students))
}

// distinctContacts counts the contacts the plan implies, keyed the way
// uq_contacts_phone is: (teacher_id, phone).
func distinctContacts(sts []resolvedStudent) int {
	seen := map[contactKey]struct{}{}
	for _, st := range sts {
		seen[contactKey{teacherID: st.teacherID, phone: st.contactPhone}] = struct{}{}
	}
	return len(seen)
}

func TestResolveBlankTeacherPhoneAnchorsOnOwner(t *testing.T) {
	t.Parallel()
	f := newFixture()
	plan, errs := f.resolve(&ParsedWorkbook{
		Classes: []ClassRow{
			{Line: 3, Name: "Lớp mới", TeacherPhone: "", StartDate: date(t, "1/9/2025"), UnitPrice: 100000, Weekday: 1, StartTime: "18:00", DurationMin: 90},
		},
		Students: []StudentRow{
			{Line: 3, StudentName: "An", ContactName: "Hùng", ContactPhone: "+84901234567", ClassName: "Lớp mới", TeacherPhone: ""},
		},
	})
	require.Empty(t, errs)
	require.Equal(t, f.owner, plan.classes[0].key.teacherID,
		"a class with no teacher belongs to the importing owner, who is a teacher too")
	require.Equal(t, f.owner, plan.students[0].teacherID)
}

func TestResolveRejectsPhoneOutsideCenter(t *testing.T) {
	t.Parallel()
	f := newFixture()
	outsider := "+84900000001"
	// The same unknown phone on four lines yields four errors, so the operator
	// fixes one cell and re-uploads once rather than four times.
	wb := &ParsedWorkbook{}
	for i := range 4 {
		wb.Classes = append(wb.Classes, ClassRow{
			Line: 3 + i, Name: "Lớp " + string(rune('A'+i)), TeacherPhone: outsider,
			StartDate: date(t, "1/9/2025"), UnitPrice: 100000, Weekday: 1, StartTime: "18:00", DurationMin: 90,
		})
	}
	_, errs := f.resolve(wb)
	require.Len(t, errs, 4)
	for _, e := range errs {
		require.Equal(t, CodeTeacherNotInCenter, e.Code)
		// The message must stay center-relative. Revealing that the number
		// exists in another center turns this into an enumeration oracle, and
		// it is exactly what a later "clearer error message" change reaches
		// for.
		require.Contains(t, e.Message, "không thuộc trung tâm của bạn")
		require.NotContains(t, e.Message, "trung tâm khác")
	}
}

func TestResolveRejectsClassFieldMismatch(t *testing.T) {
	t.Parallel()
	f := newFixture()
	_, errs := f.resolve(&ParsedWorkbook{
		Classes: []ClassRow{
			{Line: 3, Name: "Toán 9A", TeacherPhone: namPhone, StartDate: date(t, "1/9/2025"), UnitPrice: 150000, Weekday: 1, StartTime: "18:00", DurationMin: 90},
			{Line: 4, Name: "Toán 9A", TeacherPhone: namPhone, StartDate: date(t, "1/10/2025"), UnitPrice: 200000, Weekday: 4, StartTime: "18:00", DurationMin: 90},
		},
	})
	require.Len(t, errs, 2, "both the date and the price disagree")
	for _, e := range errs {
		require.Equal(t, CodeClassFieldMismatch, e.Code)
		require.Equal(t, 4, e.Line)
		require.Contains(t, e.Message, "dòng 3", "the message points at the row that set the value")
	}
}

func TestResolveRejectsDuplicateSchedule(t *testing.T) {
	t.Parallel()
	f := newFixture()
	plan, errs := f.resolve(&ParsedWorkbook{
		Classes: []ClassRow{
			{Line: 3, Name: "Toán 9A", TeacherPhone: namPhone, StartDate: date(t, "1/9/2025"), UnitPrice: 150000, Weekday: 1, StartTime: "18:00", DurationMin: 90},
			{Line: 4, Name: "Toán 9A", TeacherPhone: namPhone, StartDate: date(t, "1/9/2025"), UnitPrice: 150000, Weekday: 1, StartTime: "18:00", DurationMin: 120},
		},
	})
	require.Len(t, errs, 1)
	require.Equal(t, CodeDuplicateSchedule, errs[0].Code)
	require.Len(t, plan.classes[0].slots, 1)
}

func TestResolveRejectsClassNotInFile(t *testing.T) {
	t.Parallel()
	f := newFixture()
	_, errs := f.resolve(&ParsedWorkbook{
		Classes: []ClassRow{
			{Line: 3, Name: "Toán 9A", TeacherPhone: namPhone, StartDate: date(t, "1/9/2025"), UnitPrice: 150000, Weekday: 1, StartTime: "18:00", DurationMin: 90},
		},
		Students: []StudentRow{
			// Right class name, wrong teacher: class identity is (teacher, name),
			// because names are not unique inside a center.
			{Line: 3, StudentName: "An", ContactName: "Hùng", ContactPhone: "+84901234567", ClassName: "Toán 9A", TeacherPhone: lanPhone},
		},
	})
	require.Len(t, errs, 1)
	require.Equal(t, CodeClassNotInFile, errs[0].Code)
}

func TestResolveRejectsAmbiguousStudentButAllowsDistinguishedTwins(t *testing.T) {
	t.Parallel()
	f := newFixture()
	base := ClassRow{Line: 3, Name: "Toán 9A", TeacherPhone: namPhone, StartDate: date(t, "1/9/2025"), UnitPrice: 150000, Weekday: 1, StartTime: "18:00", DurationMin: 90}
	twin := func(note string, line int) StudentRow {
		return StudentRow{Line: line, StudentName: "Lê An", ContactName: "Lê Mai", ContactPhone: "+84977888999", ClassName: "Toán 9A", TeacherPhone: namPhone, DisplayNote: note}
	}

	_, errs := f.resolve(&ParsedWorkbook{
		Classes:  []ClassRow{base},
		Students: []StudentRow{twin("", 3), twin("", 4)},
	})
	require.Len(t, errs, 1, "two rows identical in every key column could be one child listed twice")
	require.Equal(t, CodeAmbiguousStudent, errs[0].Code)

	plan, errs := f.resolve(&ParsedWorkbook{
		Classes:  []ClassRow{base},
		Students: []StudentRow{twin("anh", 3), twin("em", 4)},
	})
	require.Empty(t, errs, "distinct notes make real twins importable")
	require.Len(t, plan.students, 2)
}

func TestResolveRejectsContactNameConflict(t *testing.T) {
	t.Parallel()
	f := newFixture()
	_, errs := f.resolve(&ParsedWorkbook{
		Classes: []ClassRow{
			{Line: 3, Name: "Toán 9A", TeacherPhone: namPhone, StartDate: date(t, "1/9/2025"), UnitPrice: 150000, Weekday: 1, StartTime: "18:00", DurationMin: 90},
		},
		Students: []StudentRow{
			{Line: 3, StudentName: "An", ContactName: "Phạm Văn Hùng", ContactPhone: "+84901234567", ClassName: "Toán 9A", TeacherPhone: namPhone},
			{Line: 4, StudentName: "Bảo", ContactName: "Phạm Thị Hoa", ContactPhone: "+84901234567", ClassName: "Toán 9A", TeacherPhone: namPhone},
		},
	})
	require.Len(t, errs, 1)
	require.Equal(t, CodeContactNameConflct, errs[0].Code)
	require.Equal(t, 4, errs[0].Line)
}

func TestResolveSameParentUnderTwoTeachersIsNotAConflict(t *testing.T) {
	t.Parallel()
	f := newFixture()
	_, errs := f.resolve(exampleRoster(t))
	require.Empty(t, errs, "the name check is per teacher, matching uq_contacts_phone")
}

func TestResolveRejectsEnrollmentBeforeClassStart(t *testing.T) {
	t.Parallel()
	f := newFixture()
	early := date(t, "1/8/2025")
	onTime := date(t, "1/10/2025")
	wb := &ParsedWorkbook{
		Classes: []ClassRow{
			{Line: 3, Name: "Toán 9A", TeacherPhone: namPhone, StartDate: date(t, "1/9/2025"), UnitPrice: 150000, Weekday: 1, StartTime: "18:00", DurationMin: 90},
		},
		Students: []StudentRow{
			{Line: 3, StudentName: "An", ContactName: "Hùng", ContactPhone: "+84901234567", ClassName: "Toán 9A", TeacherPhone: namPhone, StartedOn: &early},
			{Line: 4, StudentName: "Bảo", ContactName: "Hoa", ContactPhone: "+84901234568", ClassName: "Toán 9A", TeacherPhone: namPhone, StartedOn: &onTime},
		},
	}
	plan, errs := f.resolve(wb)
	require.Len(t, errs, 1)
	require.Equal(t, CodeEnrollBeforeStart, errs[0].Code)
	require.Equal(t, 3, errs[0].Line)
	require.Len(t, plan.students, 1)
	require.Equal(t, onTime, plan.students[0].startedOn)
}
