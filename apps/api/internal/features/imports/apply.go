package imports

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/shared/authctx"
)

// Write-stage row error codes. These name a conflict between the workbook and
// what the database already holds — as opposed to a bad cell (errors.go) or a
// bad cross-reference within the file (resolve.go).
const (
	CodeClassExistsMismatch = "CLASS_EXISTS_MISMATCH"
	CodeEnrollmentEnded     = "ENROLLMENT_ENDED"
)

// apply walks the resolved plan against the database, counting what would be
// created and what already exists. With dryRun it performs every lookup and no
// write, so the check and the real pass produce the same counts and the same
// conflicts — a clean check cannot be followed by a surprise at commit.
//
// Two anchors are in play, matching who owns what:
//
//   - Contacts and students are center data: every lookup and write runs
//     under owner — the caller's center owner with IsOwner true, built by
//     Import — so the rows anchor on the owner and dedupe center-wide no
//     matter which teacher's class a child appears under.
//   - Classes and their enrollments are pedagogical data: they anchor on the
//     class's teacher via anchorFor, whose IsOwner stays false so each class
//     reference check stays narrowed to that teacher's own rows.
func (s *Service) apply(ctx context.Context, owner authctx.Scope, plan *resolvedPlan, dryRun bool, rep *Report) ([]RowError, error) {
	var rowErrs []RowError
	classIDs := make(map[classKey]uuid.UUID, len(plan.classes))

	for _, c := range plan.classes {
		anchor := anchorFor(c.key.teacherID, owner.CenterID)
		id, errs, err := s.applyClass(ctx, anchor, c, dryRun, rep)
		if err != nil {
			return nil, err
		}
		if len(errs) > 0 {
			rowErrs = append(rowErrs, errs...)
			continue
		}
		classIDs[c.key] = id
	}
	// A class-level conflict invalidates every student row pointing at it, so
	// stop before reporting confusing follow-on errors for them.
	if len(rowErrs) > 0 {
		return rowErrs, nil
	}

	planned := newPlannedRows()
	for _, st := range plan.students {
		enrollAnchor := anchorFor(st.teacherID, owner.CenterID)
		errs, err := s.applyStudent(ctx, owner, enrollAnchor, st, classIDs[st.class], dryRun, planned, rep)
		if err != nil {
			return nil, err
		}
		rowErrs = append(rowErrs, errs...)
	}
	return rowErrs, nil
}

// plannedRows remembers what a dry run has already counted as new.
//
// A dry run writes nothing, so the second row naming the same parent still
// gets a miss from the database and would be counted as another new contact —
// two siblings would report two parents where the commit creates one and
// reuses it. The check exists to predict the commit, so it has to remember its
// own decisions. The commit needs none of this: its first row is in the
// transaction by the time the second one looks.
type plannedRows struct {
	contacts map[string]struct{}
	students map[plannedStudentKey]struct{}
}

// plannedStudentKey is the student natural key as a dry run can see it. It
// carries the contact's phone rather than its id, because a contact this run
// has only planned has no id yet. No teacher dimension: contacts and students
// anchor on the owner, so a phone-plus-name-plus-note identifies one child
// center-wide.
type plannedStudentKey struct {
	contactPhone string
	fullName     string
	displayNote  string
}

func newPlannedRows() *plannedRows {
	return &plannedRows{
		contacts: map[string]struct{}{},
		students: map[plannedStudentKey]struct{}{},
	}
}

// mark records the key and reports whether it was already there.
func (p *plannedRows) markContact(phone string) bool {
	_, seen := p.contacts[phone]
	p.contacts[phone] = struct{}{}
	return seen
}

func (p *plannedRows) markStudent(k plannedStudentKey) bool {
	_, seen := p.students[k]
	p.students[k] = struct{}{}
	return seen
}

// anchorFor builds the scope a row is written under. IsOwner stays false — see
// apply's doc comment.
func anchorFor(teacherID, centerID uuid.UUID) authctx.Scope {
	return authctx.Scope{TeacherID: teacherID, CenterID: centerID}
}

// applyClass creates or reuses one class and tops up its missing slots.
func (s *Service) applyClass(ctx context.Context, anchor authctx.Scope, c *resolvedClass, dryRun bool, rep *Report) (uuid.UUID, []RowError, error) {
	existing, found, err := s.classes.FindActiveByName(ctx, anchor, c.key.name)
	if err != nil {
		return uuid.Nil, nil, err
	}

	if !found {
		rep.Classes.Created++
		rep.Schedules.Created += len(c.slots)
		if dryRun {
			return uuid.Nil, nil, nil
		}
		created, err := s.classes.Create(ctx, anchor, classRequest(c))
		if err != nil {
			return uuid.Nil, nil, err
		}
		return created.ID, nil, nil
	}

	// The file and the database disagreeing about a class the operator is
	// re-importing is ambiguous: silently keeping the stored price would
	// invoice families at a rate nobody typed, and silently overwriting it
	// would edit a class through an endpoint whose job is to create. The
	// operator resolves it.
	if errs := classMismatches(c, existing); len(errs) > 0 {
		return uuid.Nil, errs, nil
	}
	rep.Classes.Reused++

	for _, sl := range c.slots {
		exists, err := s.classes.ScheduleExists(ctx, anchor, existing.ID,
			sl.weekday, classes.TimeOfDay(sl.startTime), c.startDate)
		if err != nil {
			return uuid.Nil, nil, err
		}
		if exists {
			rep.Schedules.Reused++
			continue
		}
		rep.Schedules.Created++
		if dryRun {
			continue
		}
		if _, err := s.classes.AddSchedule(ctx, anchor, existing.ID, scheduleRequest(sl, c.startDate)); err != nil {
			return uuid.Nil, nil, err
		}
	}
	return existing.ID, nil, nil
}

// applyStudent creates or reuses the contact, the student, and the enrollment
// behind one HocSinh row. Contact and student calls run under owner; only the
// enrollment (and the class it references) carries enrollAnchor, the class
// teacher's scope.
func (s *Service) applyStudent(ctx context.Context, owner, enrollAnchor authctx.Scope, st resolvedStudent, classID uuid.UUID, dryRun bool, planned *plannedRows, rep *Report) ([]RowError, error) {
	contactID, found, err := s.contacts.FindIDByPhone(ctx, owner, st.contactPhone)
	if err != nil {
		return nil, err
	}
	switch {
	case found:
		rep.Contacts.Reused++
	case dryRun && planned.markContact(st.contactPhone):
		// An earlier row in this same run already planned this parent, which
		// is what the commit will find when it gets here.
		rep.Contacts.Reused++
	default:
		rep.Contacts.Created++
		if !dryRun {
			row, err := s.contacts.Create(ctx, owner, contacts.CreateRequest{
				FullName: st.contactName,
				Phone:    st.contactPhone,
			})
			if err != nil {
				return nil, err
			}
			contactID = row.ID
		}
	}

	var note *string
	if st.displayNote != "" {
		note = &st.displayNote
	}

	studentID := uuid.Nil
	if contactID != uuid.Nil {
		studentID, found, err = s.students.FindIDByName(ctx, owner, contactID, st.studentName, note)
		if err != nil {
			return nil, err
		}
	} else {
		// Dry run with a contact that does not exist yet: the student cannot
		// exist either, since a student is reached through its contact.
		found = false
	}
	switch {
	case found:
		rep.Students.Reused++
	case dryRun && planned.markStudent(plannedStudentKey{
		contactPhone: st.contactPhone,
		fullName:     st.studentName,
		displayNote:  st.displayNote,
	}):
		// The same child listed in two classes: one student row, two
		// enrollments. Only the check has to work this out for itself.
		rep.Students.Reused++
	default:
		rep.Students.Created++
		if !dryRun {
			row, err := s.students.Create(ctx, owner, students.CreateRequest{
				FullName:    st.studentName,
				ContactID:   contactID,
				DisplayNote: st.displayNote,
			})
			if err != nil {
				return nil, err
			}
			studentID = row.ID
		}
	}

	if studentID == uuid.Nil || classID == uuid.Nil {
		// Same reasoning: a brand-new student has no enrollment history.
		rep.Enrollments.Created++
		return nil, nil
	}

	existing, found, err := s.enrollments.FindByStudentAndClass(ctx, enrollAnchor, studentID, classID)
	if err != nil {
		return nil, err
	}
	if found {
		if existing.EndedOn != nil {
			// uq_enrollments_active only covers open rows, so this one is
			// invisible to the database. Re-creating it would set started_on
			// back to the class start date and make the student retroactively
			// present for every session since — months of attendance and
			// invoices for a child who left. Re-admitting is a deliberate act.
			return []RowError{rowErr(SheetStudents, st.line, studentHeaders[colStudentName],
				CodeEnrollmentEnded,
				"học sinh này đã nghỉ lớp từ %s; nhận lại phải làm trong ứng dụng, không qua import",
				existing.EndedOn.Format(sheetDateLayout))}, nil
		}
		rep.Enrollments.Reused++
		return nil, nil
	}

	rep.Enrollments.Created++
	if dryRun {
		return nil, nil
	}
	_, err = s.enrollments.Create(ctx, enrollAnchor, enrollments.CreateRequest{
		StudentID: studentID,
		ClassID:   classID,
		StartedOn: st.startedOn.Format(dateWireLayout),
	})
	return nil, err
}

// dateWireLayout is the form the four feature DTOs bind dates in.
const dateWireLayout = "2006-01-02"

// classMismatches compares a reused class's stored fields against the file.
func classMismatches(c *resolvedClass, existing *classes.Class) []RowError {
	var out []RowError
	add := func(column, stored, inFile string) {
		out = append(out, rowErr(SheetClasses, c.firstLine, column, CodeClassExistsMismatch,
			"lớp %q đã có trong hệ thống với %s là %s, file ghi %s",
			c.key.name, column, stored, inFile))
	}
	if !existing.StartDate.Equal(c.startDate) {
		add(classHeaders[colClassStartDate],
			existing.StartDate.Format(sheetDateLayout), c.startDate.Format(sheetDateLayout))
	}
	if existing.DefaultUnitPrice != c.unitPrice {
		add(classHeaders[colClassUnitPrice],
			strconv.FormatInt(existing.DefaultUnitPrice, 10), strconv.FormatInt(c.unitPrice, 10))
	}
	if !sameDate(existing.EndDate, c.endDate) {
		add(classHeaders[colClassEndDate], formatDatePtr(existing.EndDate), formatDatePtr(c.endDate))
	}
	return out
}

// classRequest builds the create payload for a new class and all its slots.
func classRequest(c *resolvedClass) classes.CreateClassRequest {
	price := c.unitPrice
	req := classes.CreateClassRequest{
		Name:             c.key.name,
		StartDate:        c.startDate.Format(dateWireLayout),
		DefaultUnitPrice: &price,
		Schedules:        make([]classes.ScheduleRequest, 0, len(c.slots)),
	}
	if c.endDate != nil {
		req.EndDate = c.endDate.Format(dateWireLayout)
	}
	for _, sl := range c.slots {
		req.Schedules = append(req.Schedules, scheduleRequest(sl, c.startDate))
	}
	return req
}

// scheduleRequest builds one slot payload.
//
// EffectiveFrom is always set explicitly, never left blank. AddSchedule
// defaults a blank one to the STORED class's start_date while ScheduleExists
// was asked with the file's date; if those ever diverged, every re-import
// would append another identical slot forever, and class_schedules has no
// unique index to catch it.
func scheduleRequest(sl slot, effectiveFrom time.Time) classes.ScheduleRequest {
	weekday := sl.weekday
	return classes.ScheduleRequest{
		Weekday:       &weekday,
		StartTime:     sl.startTime,
		DurationMin:   sl.durationMin,
		EffectiveFrom: effectiveFrom.Format(dateWireLayout),
	}
}
