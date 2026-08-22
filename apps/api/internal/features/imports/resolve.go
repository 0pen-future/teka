package imports

import (
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Resolution-stage row error codes. Each names a cross-reference the operator
// got wrong, as opposed to a badly formatted cell (see errors.go).
const (
	CodeTeacherNotInCenter = "TEACHER_NOT_IN_CENTER"
	CodeClassNotInFile     = "CLASS_NOT_IN_FILE"
	CodeClassFieldMismatch = "CLASS_FIELD_MISMATCH"
	CodeDuplicateSchedule  = "DUPLICATE_SCHEDULE"
	CodeAmbiguousStudent   = "AMBIGUOUS_STUDENT"
	CodeContactNameConflct = "CONTACT_NAME_CONFLICT"
	CodeEnrollBeforeStart  = "ENROLL_BEFORE_CLASS_START"
)

// classKey identifies one class within a center: the teacher who runs it plus
// its name. Class names are not unique per center, so the teacher is part of
// the identity, not a detail hanging off it.
type classKey struct {
	teacherID uuid.UUID
	name      string
}

// slot is one weekly timetable entry of a resolved class.
type slot struct {
	line        int
	weekday     int16
	startTime   string
	durationMin int16
}

// resolvedClass is one class's worth of Lop rows, grouped and cross-checked.
// StartDate, UnitPrice and EndDate are class-level: every row of the group had
// to agree on them.
type resolvedClass struct {
	key       classKey
	firstLine int
	startDate time.Time
	unitPrice int64
	endDate   *time.Time
	slots     []slot
}

// resolvedStudent is one HocSinh row with its class reference resolved.
type resolvedStudent struct {
	line         int
	teacherID    uuid.UUID
	class        classKey
	studentName  string
	contactName  string
	contactPhone string
	displayNote  string
	startedOn    time.Time
}

// resolvedPlan is the fully cross-checked import, ready to be written or
// counted. Classes keeps file order so the report and any error messages read
// in the same order as the operator's sheet.
type resolvedPlan struct {
	classes  []*resolvedClass
	students []resolvedStudent
}

// resolve turns coerced rows into a cross-checked plan. Every phone has been
// mapped through the caller's own center directory, every HocSinh row points
// at a class defined in the same file, and every class-level field is
// consistent across the rows that describe it.
//
// ownerID anchors rows whose teacher-phone cell was left blank. dir is the
// caller's center directory: a phone absent from it does not resolve, and that
// absence is the authorization result — there is no global lookup here and
// must never be one.
func resolve(wb *ParsedWorkbook, dir map[string]uuid.UUID, ownerID uuid.UUID) (*resolvedPlan, []RowError) {
	var errs []RowError

	classes, classErrs := resolveClasses(wb.Classes, dir, ownerID)
	errs = append(errs, classErrs...)

	byKey := make(map[classKey]*resolvedClass, len(classes))
	for _, c := range classes {
		byKey[c.key] = c
	}

	students, studentErrs := resolveStudents(wb.Students, dir, ownerID, byKey)
	errs = append(errs, studentErrs...)

	return &resolvedPlan{classes: classes, students: students}, errs
}

// resolveTeacher maps a row's teacher-phone cell to a teacher id. A blank cell
// means the importing owner — the workbook is allowed to describe a class
// nobody has been assigned yet, and the owner is a teacher too.
//
// The failure message stays center-relative on purpose. Telling the operator
// that the number belongs to some other center would turn this endpoint into
// an account-enumeration oracle, and it is the exact wording a later "better
// error message" change will reach for.
func resolveTeacher(sheet string, line int, column, phone string, dir map[string]uuid.UUID, ownerID uuid.UUID) (uuid.UUID, *RowError) {
	if phone == "" {
		return ownerID, nil
	}
	id, ok := dir[phone]
	if !ok {
		e := rowErr(sheet, line, column, CodeTeacherNotInCenter,
			"số điện thoại này không thuộc trung tâm của bạn")
		return uuid.Nil, &e
	}
	return id, nil
}

// resolveClasses groups Lop rows into classes and checks that the rows
// describing one class agree about it.
func resolveClasses(rows []ClassRow, dir map[string]uuid.UUID, ownerID uuid.UUID) ([]*resolvedClass, []RowError) {
	var errs []RowError
	var order []*resolvedClass
	byKey := make(map[classKey]*resolvedClass)

	for _, r := range rows {
		teacherID, rerr := resolveTeacher(SheetClasses, r.Line,
			classHeaders[colClassTeacherPhone], r.TeacherPhone, dir, ownerID)
		if rerr != nil {
			errs = append(errs, *rerr)
			continue
		}

		key := classKey{teacherID: teacherID, name: r.Name}
		cur, seen := byKey[key]
		if !seen {
			cur = &resolvedClass{
				key:       key,
				firstLine: r.Line,
				startDate: r.StartDate,
				unitPrice: r.UnitPrice,
				endDate:   r.EndDate,
			}
			byKey[key] = cur
			order = append(order, cur)
		} else if mismatches := classFieldMismatches(cur, r); len(mismatches) > 0 {
			// The rows of one group describe a single class, so disagreeing
			// about its start date or price means the file states two
			// different truths and neither can be chosen for the operator.
			for _, m := range mismatches {
				errs = append(errs, rowErr(SheetClasses, r.Line, m.column, CodeClassFieldMismatch,
					"lớp %q ở dòng %d ghi %s là %s, dòng này ghi %s",
					r.Name, cur.firstLine, m.column, m.first, m.here))
			}
			continue
		}

		if dup := findSlot(cur.slots, r.Weekday, r.StartTime); dup != nil {
			errs = append(errs, rowErr(SheetClasses, r.Line, classHeaders[colClassWeekday],
				CodeDuplicateSchedule, "lớp %q đã có buổi này ở dòng %d", r.Name, dup.line))
			continue
		}
		cur.slots = append(cur.slots, slot{
			line:        r.Line,
			weekday:     r.Weekday,
			startTime:   r.StartTime,
			durationMin: r.DurationMin,
		})
	}
	return order, errs
}

type fieldMismatch struct {
	column string
	first  string
	here   string
}

// classFieldMismatches compares a later row's class-level fields against the
// first row of its group.
func classFieldMismatches(c *resolvedClass, r ClassRow) []fieldMismatch {
	var out []fieldMismatch
	if !c.startDate.Equal(r.StartDate) {
		out = append(out, fieldMismatch{
			column: classHeaders[colClassStartDate],
			first:  c.startDate.Format(sheetDateLayout),
			here:   r.StartDate.Format(sheetDateLayout),
		})
	}
	if c.unitPrice != r.UnitPrice {
		out = append(out, fieldMismatch{
			column: classHeaders[colClassUnitPrice],
			first:  strconv.FormatInt(c.unitPrice, 10),
			here:   strconv.FormatInt(r.UnitPrice, 10),
		})
	}
	if !sameDate(c.endDate, r.EndDate) {
		out = append(out, fieldMismatch{
			column: classHeaders[colClassEndDate],
			first:  formatDatePtr(c.endDate),
			here:   formatDatePtr(r.EndDate),
		})
	}
	return out
}

func findSlot(slots []slot, weekday int16, startTime string) *slot {
	for i := range slots {
		if slots[i].weekday == weekday && slots[i].startTime == startTime {
			return &slots[i]
		}
	}
	return nil
}

// studentKey is the identity a re-import must recognise: the same child, of
// the same parent, in the same class. The note is what separates twins.
type studentKey struct {
	class        classKey
	contactPhone string
	studentName  string
	displayNote  string
}

// resolveStudents maps each HocSinh row onto a class from the same file and
// checks the rows are mutually distinguishable.
func resolveStudents(rows []StudentRow, dir map[string]uuid.UUID, ownerID uuid.UUID, classes map[classKey]*resolvedClass) ([]resolvedStudent, []RowError) {
	var errs []RowError
	out := make([]resolvedStudent, 0, len(rows))
	seen := make(map[studentKey]int, len(rows))
	contactNames := make(map[contactKey]nameAt, len(rows))

	for _, r := range rows {
		teacherID, rerr := resolveTeacher(SheetStudents, r.Line,
			studentHeaders[colStudentTeacherPhone], r.TeacherPhone, dir, ownerID)
		if rerr != nil {
			errs = append(errs, *rerr)
			continue
		}

		key := classKey{teacherID: teacherID, name: r.ClassName}
		class, ok := classes[key]
		if !ok {
			// Deliberately not resolved against classes already in the
			// database: class names are not unique per center, so picking one
			// would be a guess. Sheet 1 is the only source of classes.
			errs = append(errs, rowErr(SheetStudents, r.Line, studentHeaders[colStudentClassName],
				CodeClassNotInFile, "sheet %q không có lớp %q của giáo viên này", SheetClasses, r.ClassName))
			continue
		}

		// One phone is one parent. Two spellings of the name under the same
		// teacher means the file is ambiguous about who owns the number.
		ck := contactKey{teacherID: teacherID, phone: r.ContactPhone}
		if prev, dup := contactNames[ck]; dup && prev.name != r.ContactName {
			errs = append(errs, rowErr(SheetStudents, r.Line, studentHeaders[colContactName],
				CodeContactNameConflct, "số %s đã ghi tên phụ huynh là %q ở dòng %d",
				r.ContactPhone, prev.name, prev.line))
			continue
		}
		contactNames[ck] = nameAt{name: r.ContactName, line: r.Line}

		sk := studentKey{class: key, contactPhone: r.ContactPhone, studentName: r.StudentName, displayNote: r.DisplayNote}
		if prevLine, dup := seen[sk]; dup {
			// Indistinguishable from the same child listed twice. Twins need a
			// Ghi chú phân biệt, otherwise a re-import cannot tell one row
			// from two.
			errs = append(errs, rowErr(SheetStudents, r.Line, studentHeaders[colStudentDisplayNote],
				CodeAmbiguousStudent,
				"trùng hoàn toàn với dòng %d; nếu là hai học sinh khác nhau, ghi chú phân biệt cho mỗi em", prevLine))
			continue
		}
		seen[sk] = r.Line

		startedOn := class.startDate
		if r.StartedOn != nil {
			// started_on means "on the roster from", and ActiveOn selects
			// sessions with started_on <= session_date. A date before the
			// class began would silently bill sessions the student never
			// attended, and is almost always a typo — early registration is
			// recorded as the class start date, so no real case is lost.
			if r.StartedOn.Before(class.startDate) {
				errs = append(errs, rowErr(SheetStudents, r.Line, studentHeaders[colStudentStartedOn],
					CodeEnrollBeforeStart, "trước ngày khai giảng %s của lớp %q",
					class.startDate.Format(sheetDateLayout), r.ClassName))
				continue
			}
			startedOn = *r.StartedOn
		}

		out = append(out, resolvedStudent{
			line:         r.Line,
			teacherID:    teacherID,
			class:        key,
			studentName:  r.StudentName,
			contactName:  r.ContactName,
			contactPhone: r.ContactPhone,
			displayNote:  r.DisplayNote,
			startedOn:    startedOn,
		})
	}
	return out, errs
}

type contactKey struct {
	teacherID uuid.UUID
	phone     string
}

type nameAt struct {
	name string
	line int
}

func sameDate(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Equal(*b)
	}
}

func formatDatePtr(t *time.Time) string {
	if t == nil {
		return "trống"
	}
	return t.Format(sheetDateLayout)
}
