package classes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classcode"
	"teka/apps/api/internal/shared/dbtypes"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// StaffSeeder is what classes needs from the class-staff feature: maintaining
// the class_staff mirror of classes.teacher_id (the dual-write invariant:
// whoever teacher_id names must hold the class's one active giao_vien stint),
// and loading the caller's active roles for the readable ports.
// classstaff.Repository satisfies it structurally; classes must not import
// classstaff (classstaff reads classes for its existence gate).
type StaffSeeder interface {
	SyncPrimaryTeacher(ctx context.Context, classID, centerID, teacherID uuid.UUID) error
	RolesByClass(ctx context.Context, teacherID, centerID uuid.UUID, classIDs []uuid.UUID) (map[uuid.UUID][]string, error)
}

// Service owns class business rules: atomic create-with-schedules, the
// archive-vs-delete distinction, and effective-range schedule management.
type Service struct {
	repo  Repository
	tx    database.TxManager
	staff StaffSeeder
}

// NewService builds the classes service. staff is required: every class must
// be born with its teacher's giao_vien stint, or assignment-based read
// scoping would silently exclude the primary teacher.
func NewService(repo Repository, tx database.TxManager, staff StaffSeeder) *Service {
	return &Service{repo: repo, tx: tx, staff: staff}
}

// Create inserts the class and its schedule rows as the caller's own —
// including an owner, who creates rows as themselves, never on behalf of
// another teacher.
func (s *Service) Create(ctx context.Context, sc authctx.Scope, req CreateClassRequest) (*Class, error) {
	return s.CreateAnchored(ctx, sc.Self(), req)
}

// CreateAnchored inserts the class and its schedule rows in one transaction
// anchored on a — a failing schedule insert leaves no class row. Schedules
// with no effective_from inherit the class start date. It exists for the
// roster import: each class anchors to the teacher its own workbook row
// names, via a resolved from that name, not from the importing caller.
func (s *Service) CreateAnchored(ctx context.Context, a authctx.Anchor, req CreateClassRequest) (*Class, error) {
	startDate, err := parseDate("start_date", req.StartDate)
	if err != nil {
		return nil, err
	}
	endDate, err := parseDatePtr("end_date", req.EndDate)
	if err != nil {
		return nil, err
	}

	code, err := s.resolveCode(ctx, a, req.Code, uuid.Nil)
	if err != nil {
		return nil, err
	}

	class := &Class{
		ID:        id.New(),
		TeacherID: a.TeacherID,
		CenterID:  a.CenterID,
		Name:      req.Name,
		StartDate: startDate,
		EndDate:   endDate,
		Status:    StatusActive,
		Code:      code,
		Tags:      tagList(req.Tags),
		Note:      noteValue(req.Note),
	}
	schedules := make([]Schedule, len(req.Schedules))
	for i, sr := range req.Schedules {
		schedule, err := scheduleFromRequest(a.TeacherID, a.CenterID, class.ID, startDate, sr)
		if err != nil {
			return nil, err
		}
		schedules[i] = *schedule
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// The course is resolved (and share-locked) inside the write
		// transaction so a concurrent course delete cannot slip between
		// the lookup and the insert.
		course, err := s.resolveCourse(ctx, a, req.CourseID, nil)
		if err != nil {
			return err
		}
		// The price is required unless a course supplies its default.
		unitPrice, err := unitPriceFor(req.DefaultUnitPrice, course)
		if err != nil {
			return err
		}
		class.DefaultUnitPrice = unitPrice
		class.Course = course
		if course != nil {
			class.CourseID = &course.ID
		}
		if err := s.repo.CreateWithSchedules(ctx, class, schedules); err != nil {
			return err
		}
		// Same transaction as the class row: teacher_id and the giao_vien
		// stint must never be observable apart.
		return s.staff.SyncPrimaryTeacher(ctx, class.ID, class.CenterID, class.TeacherID)
	})
	if err != nil {
		return nil, codeConflictOr(err, class.Code)
	}
	class.Schedules = schedules
	return class, nil
}

// codeAttempts bounds the generate-and-probe loop for a blank code. With
// 32^6 values per center a second collision is already implausible, so three
// misses mean something other than chance is wrong and the caller is told.
const codeAttempts = 3

// resolveCode returns the code a class row should carry: the requested one,
// checked for shape and for a clash with another live class in the center
// (exceptID excludes the row being edited), or a freshly minted one when the
// request left it blank. A clash — explicit or after every generate attempt
// — is ErrCodeTaken wrapped as a 409 so the form can point at its code field.
func (s *Service) resolveCode(ctx context.Context, a authctx.Anchor, requested *string, exceptID uuid.UUID) (string, error) {
	if requested != nil && strings.TrimSpace(*requested) != "" {
		code := strings.TrimSpace(*requested)
		if !classcode.Valid(code) {
			return "", apperror.Invalid("validation failed",
				map[string]string{"code": "must be 2-20 characters of A-Z, 0-9 or -"})
		}
		taken, err := s.repo.CodeExists(ctx, a, code, exceptID)
		if err != nil {
			return "", err
		}
		if taken {
			return "", codeTakenError(code)
		}
		return code, nil
	}
	for range codeAttempts {
		code := classcode.Generate()
		taken, err := s.repo.CodeExists(ctx, a, code, exceptID)
		if err != nil {
			return "", err
		}
		if !taken {
			return code, nil
		}
	}
	return "", codeTakenError("")
}

func codeTakenError(code string) error {
	msg := "could not allocate a free class code; retry or choose one"
	appErr := apperror.New(CodeClassCodeTaken, http.StatusConflict, msg)
	if code != "" {
		msg = fmt.Sprintf("class code %q is already used by another class in this center", code)
		appErr = apperror.New(CodeClassCodeTaken, http.StatusConflict, msg)
		appErr.Fields = map[string]string{"code": msg}
	}
	appErr.Err = ErrCodeTaken
	return appErr
}

// codeConflictOr maps a duplicate-key error from the class insert or update
// to the same 409 the CodeExists probe produces. The probe and the partial
// unique index on (center_id, code) can disagree under a concurrent write of
// the same code; the index is the authority, and its refusal must read as
// "code taken", not as an internal failure.
func codeConflictOr(err error, code string) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return codeTakenError(code)
	}
	return err
}

// tagList copies the request tags into the JSONB list, never nil so the
// column always holds an array.
func tagList(tags []string) dbtypes.StringList {
	out := make(dbtypes.StringList, 0, len(tags))
	return append(out, tags...)
}

// noteValue stores a blank note as NULL rather than an empty string.
func noteValue(note *string) *string {
	if note == nil || *note == "" {
		return nil
	}
	return note
}

// scheduleFromRequest builds a schedule row, defaulting effective_from to the
// class start date. teacherID and centerID are the row's own tenant anchors —
// for a new class these are the caller's scope, but for a schedule added to
// an existing class (AddSchedule) they must be the parent class's own
// anchors, since an owner may be adding to a member's class.
func scheduleFromRequest(teacherID, centerID, classID uuid.UUID, startDate time.Time, sr ScheduleRequest) (*Schedule, error) {
	effectiveFrom := startDate
	if sr.EffectiveFrom != "" {
		parsed, err := parseDate("effective_from", sr.EffectiveFrom)
		if err != nil {
			return nil, err
		}
		effectiveFrom = parsed
	}
	effectiveTo, err := parseDatePtr("effective_to", sr.EffectiveTo)
	if err != nil {
		return nil, err
	}
	return &Schedule{
		ID:            id.New(),
		TeacherID:     teacherID,
		CenterID:      centerID,
		ClassID:       classID,
		Weekday:       *sr.Weekday,
		StartTime:     TimeOfDay(sr.StartTime),
		DurationMin:   sr.DurationMin,
		EffectiveFrom: effectiveFrom,
		EffectiveTo:   effectiveTo,
	}, nil
}

// Get returns one class with its schedules; archived classes stay
// retrievable by id.
func (s *Service) Get(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*Class, error) {
	class, err := s.repo.GetByID(ctx, sc, classID)
	if err != nil {
		return nil, translate(err)
	}
	return class, nil
}

// List returns a page of classes. The default filter (active only) matches
// the idx_classes_teacher partial-index predicate.
func (s *Service) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, int64, error) {
	return s.repo.List(ctx, sc, filter, p)
}

// GetReadable is Get's READ-port counterpart: any class the caller holds a
// class_staff stint on (ended included — history reads) or can see center-wide.
// Read-only consumers that never branch on the caller's roles use this variant
// so a plain readable-resolution costs one query; every write path keeps
// resolving through Get or GetWritable, the shared write gates.
func (s *Service) GetReadable(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*Class, error) {
	class, err := s.repo.GetReadableByID(ctx, sc, classID)
	if err != nil {
		return nil, translate(err)
	}
	return class, nil
}

// GetReadableWithRoles is GetReadable plus the caller's ACTIVE role keys on
// the class, for consumers that branch on them: the class detail response's
// my_staff_roles and the classbook's write-capability probe. Kept separate so
// consumers that discard the roles never pay the extra query.
func (s *Service) GetReadableWithRoles(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*Class, []string, error) {
	class, err := s.GetReadable(ctx, sc, classID)
	if err != nil {
		return nil, nil, err
	}
	roles, err := s.staff.RolesByClass(ctx, sc.TeacherID, sc.CenterID, []uuid.UUID{classID})
	if err != nil {
		return nil, nil, err
	}
	return class, roles[classID], nil
}

// GetWritable is the capability write gate: it resolves the writing roles for
// capability from the authctx map and fetches the class through the write
// port (active stint in those roles, or center-wide scope). When the write
// fetch misses, the read port disambiguates the honest answer: readable means
// the caller has SOME relationship to the class (an ended stint, a different
// role, or creator rows) but not this capability → 403; unreadable → 404, so
// outsiders cannot probe class ids.
func (s *Service) GetWritable(ctx context.Context, sc authctx.Scope, classID uuid.UUID, capability authctx.ClassCapability) (*Class, error) {
	roles := authctx.StaffRolesFor(capability)
	class, err := s.repo.GetWritableByID(ctx, sc, classID, roles)
	if err == nil {
		return class, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if _, rerr := s.repo.GetReadableByID(ctx, sc, classID); rerr == nil {
		return nil, apperror.Forbidden("your role on this class does not allow this action")
	} else if !errors.Is(rerr, ErrNotFound) {
		return nil, rerr
	}
	return nil, apperror.NotFound("class")
}

// ListReadable is List's READ-port counterpart. Roles come from one batch
// query over the page's class ids, keyed by class id — never per row.
func (s *Service) ListReadable(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, map[uuid.UUID][]string, int64, error) {
	rows, total, err := s.repo.ListReadable(ctx, sc, filter, p)
	if err != nil {
		return nil, nil, 0, err
	}
	roles, err := s.staff.RolesByClass(ctx, sc.TeacherID, sc.CenterID, ClassIDs(rows))
	if err != nil {
		return nil, nil, 0, err
	}
	return rows, roles, total, nil
}

// ClassIDs collects the ids of a page of classes, in order.
func ClassIDs(rows []Class) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	return ids
}

// StudentCounts returns the open enrollment count per class id, keyed by id
// (absent = 0), for classes the caller already read through a readable port.
// Kept off GetReadableWithRoles/ListReadable so consumers that never render
// a count (the sessions write probe) do not pay the query.
func (s *Service) StudentCounts(ctx context.Context, sc authctx.Scope, classIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return s.repo.CountActiveEnrollmentsByClass(ctx, sc, classIDs)
}

// Update edits the class's own fields; status and schedules have their own
// endpoints.
func (s *Service) Update(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req UpdateClassRequest) (*Class, error) {
	class, err := s.repo.GetByID(ctx, sc, classID)
	if err != nil {
		return nil, translate(err)
	}
	startDate, err := parseDate("start_date", req.StartDate)
	if err != nil {
		return nil, err
	}
	endDate, err := parseDatePtr("end_date", req.EndDate)
	if err != nil {
		return nil, err
	}
	class.Name = req.Name
	class.StartDate = startDate
	class.EndDate = endDate
	class.DefaultUnitPrice = *req.DefaultUnitPrice
	// The catalog fields patch: nil keeps the stored value, a present pointer
	// replaces it whole. A blank code also keeps the stored one — a class
	// never goes back to having no code.
	if req.Code != nil && strings.TrimSpace(*req.Code) != "" {
		code, err := s.resolveCode(ctx, authctx.Anchor{TeacherID: class.TeacherID, CenterID: class.CenterID}, req.Code, class.ID)
		if err != nil {
			return nil, err
		}
		class.Code = code
	}
	if req.Tags != nil {
		class.Tags = tagList(*req.Tags)
	}
	if req.Recruiting != nil {
		class.Recruiting = *req.Recruiting
	}
	if req.Note != nil {
		class.Note = noteValue(req.Note)
	}
	if req.LineageNote != nil {
		class.LineageNote = noteValue(req.LineageNote)
	}
	if req.ParentClassID != nil {
		parentID, err := s.resolveParentClass(ctx, authctx.Anchor{TeacherID: class.TeacherID, CenterID: class.CenterID}, req.ParentClassID, class.ID)
		if err != nil {
			return nil, err
		}
		class.ParentClassID = parentID
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if req.CourseID != nil {
			// Resolved under the same transaction as the write so the
			// share lock on the course holds until the class row lands.
			course, err := s.resolveCourse(ctx, authctx.Anchor{TeacherID: class.TeacherID, CenterID: class.CenterID}, req.CourseID, class.CourseID)
			if err != nil {
				return err
			}
			class.Course, class.CourseID = course, nil
			if course != nil {
				class.CourseID = &course.ID
			}
		}
		return s.repo.Update(ctx, class)
	})
	if err != nil {
		return nil, codeConflictOr(err, class.Code)
	}
	return class, nil
}

// resolveCourse turns a request's course_id into the center's course row:
// nil or blank means no course; anything else must name a live course of
// the anchor's center, else 422 on the course_id field. The field carries
// no uuid binding tag (a pointer to "" would fail it), so the shape is
// checked here and a bad one is the same 422. An archived course takes no
// new class: current is the class's stored course id, and resending it is
// not a new attachment, so an already-attached class keeps saving.
func (s *Service) resolveCourse(ctx context.Context, a authctx.Anchor, raw *string, current *uuid.UUID) (*CourseRef, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	invalid := apperror.Invalid("Khóa học không tồn tại trong trung tâm",
		map[string]string{"course_id": "phải là khóa học còn hiệu lực của trung tâm"})
	courseID, err := uuid.Parse(strings.TrimSpace(*raw))
	if err != nil {
		return nil, invalid
	}
	course, err := s.repo.FindCourse(ctx, a, courseID)
	if errors.Is(err, ErrCourseNotFound) {
		return nil, invalid
	}
	if err != nil {
		return nil, err
	}
	if course.Status == courseStatusArchived && (current == nil || *current != course.ID) {
		return nil, apperror.Invalid("Khóa học đã ngừng tuyển, không gắn lớp mới",
			map[string]string{"course_id": "khóa học đã ngừng tuyển"})
	}
	return course, nil
}

// resolveParentClass turns a request's parent_class_id into the linked
// class id: blank means no parent; anything else must name a live class of
// the anchor's center other than the class itself, else 422 on the field.
func (s *Service) resolveParentClass(ctx context.Context, a authctx.Anchor, raw *string, selfID uuid.UUID) (*uuid.UUID, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	invalid := apperror.Invalid("Lớp gốc không tồn tại trong trung tâm",
		map[string]string{"parent_class_id": "phải là lớp còn hiệu lực của trung tâm, khác lớp hiện tại"})
	parentID, err := uuid.Parse(strings.TrimSpace(*raw))
	if err != nil || parentID == selfID {
		return nil, invalid
	}
	exists, err := s.repo.LiveClassInCenter(ctx, a, parentID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, invalid
	}
	cycle, err := s.repo.ParentCreatesCycle(ctx, a, parentID, selfID)
	if err != nil {
		return nil, err
	}
	if cycle {
		return nil, apperror.Invalid("Lớp gốc tạo vòng lặp trong lịch sử lớp",
			map[string]string{"parent_class_id": "không được tạo vòng lặp trong lịch sử tách lớp"})
	}
	return &parentID, nil
}

// unitPriceFor picks the class's per-session price: the request's own value
// wins, then the course's default; with neither the field is required.
func unitPriceFor(requested *int64, course *CourseRef) (int64, error) {
	if requested != nil {
		return *requested, nil
	}
	if course != nil {
		return course.DefaultUnitPrice, nil
	}
	return 0, apperror.Invalid("validation failed",
		map[string]string{"default_unit_price": "required unless course_id is set"})
}

// Stats buckets the classes the caller can read by phase on today. It
// shares ListReadable's port, so the chips never count a class the list
// would not show.
func (s *Service) Stats(ctx context.Context, sc authctx.Scope, today time.Time) (ClassStatsResponse, error) {
	return s.repo.CountReadableByPhase(ctx, sc, today)
}

// Archive flips the class to archived — the normal end-of-term action, and
// idempotent.
func (s *Service) Archive(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*Class, error) {
	if err := s.repo.Archive(ctx, sc, classID); err != nil {
		return nil, translate(err)
	}
	return s.Get(ctx, sc, classID)
}

// Delete soft-deletes a class created by mistake. A class students are still
// enrolled in refuses with 409 and points at archiving instead — soft delete
// would leave live enrollments dangling on an invisible class.
func (s *Service) Delete(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, sc, classID); err != nil {
		return translate(err)
	}
	open, err := s.repo.CountOpenEnrollments(ctx, sc, classID)
	if err != nil {
		return err
	}
	if open > 0 {
		appErr := apperror.Conflict(fmt.Sprintf(
			"class has %d open enrollment(s); archive the class instead of deleting it", open))
		appErr.Err = ErrHasOpenEnrollments
		return appErr
	}
	return translate(s.repo.SoftDelete(ctx, sc, classID))
}

// addSchedule is the shared core behind AddSchedule and AddScheduleAnchored:
// it builds one schedule row anchored on a — startDate defaults a blank
// effective_from — and inserts it.
func (s *Service) addSchedule(ctx context.Context, a authctx.Anchor, classID uuid.UUID, startDate time.Time, req ScheduleRequest) (*Schedule, error) {
	schedule, err := scheduleFromRequest(a.TeacherID, a.CenterID, classID, startDate, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.AddSchedule(ctx, schedule); err != nil {
		return nil, err
	}
	return schedule, nil
}

// AddSchedule appends a timetable row to the class — the second half of the
// close-and-replace flow for changing a weekly slot. The new row inherits the
// parent class's own teacher and center, not the caller's scope: an owner
// adding a schedule to a member's class must not stamp themselves as its
// teacher.
func (s *Service) AddSchedule(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req ScheduleRequest) (*Schedule, error) {
	class, err := s.repo.GetByID(ctx, sc, classID)
	if err != nil {
		return nil, translate(err)
	}
	return s.addSchedule(ctx, authctx.Anchor{TeacherID: class.TeacherID, CenterID: class.CenterID}, classID, class.StartDate, req)
}

// AddScheduleAnchored is AddSchedule's import counterpart: the caller already
// resolved the class (FindActiveByName) and holds its own anchor and start
// date directly, so this skips the GetByID re-fetch AddSchedule needs to
// learn them.
func (s *Service) AddScheduleAnchored(ctx context.Context, a authctx.Anchor, classID uuid.UUID, startDate time.Time, req ScheduleRequest) (*Schedule, error) {
	return s.addSchedule(ctx, a, classID, startDate, req)
}

// UpdateSchedule edits one row in place — for correcting a mistake or closing
// the row by setting effective_to. Real timetable changes should close the
// old row and add a new one.
func (s *Service) UpdateSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID, req UpdateScheduleRequest) (*Schedule, error) {
	schedule, err := s.repo.GetSchedule(ctx, sc, classID, scheduleID)
	if err != nil {
		return nil, translate(err)
	}
	effectiveFrom, err := parseDate("effective_from", req.EffectiveFrom)
	if err != nil {
		return nil, err
	}
	effectiveTo, err := parseDatePtr("effective_to", req.EffectiveTo)
	if err != nil {
		return nil, err
	}
	schedule.Weekday = *req.Weekday
	schedule.StartTime = TimeOfDay(req.StartTime)
	schedule.DurationMin = req.DurationMin
	schedule.EffectiveFrom = effectiveFrom
	schedule.EffectiveTo = effectiveTo
	if err := s.repo.UpdateSchedule(ctx, schedule); err != nil {
		return nil, err
	}
	return schedule, nil
}

// DeleteSchedule soft-deletes a timetable row.
func (s *Service) DeleteSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) error {
	return translate(s.repo.SoftDeleteSchedule(ctx, sc, classID, scheduleID))
}

// ListEffectiveSchedules exposes the session-generation contract: rows whose
// effective range intersects [from, to]. It is a read, resolved through the
// read port: whoever may see the class may see its timetable.
func (s *Service) ListEffectiveSchedules(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Schedule, error) {
	if _, err := s.repo.GetReadableByID(ctx, sc, classID); err != nil {
		return nil, translate(err)
	}
	return s.repo.ListEffectiveSchedules(ctx, sc, classID, from, to)
}

// translate maps domain errors onto the API error contract, keeping the
// domain error as the cause so errors.Is still works.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("class")
	case errors.Is(err, ErrScheduleNotFound):
		return apperror.NotFound("schedule")
	default:
		return err
	}
}

// FindActiveByName resolves a live, active class by its exact name under one
// anchor. It is the roster import's create-or-reuse seam; classes.name
// carries no unique index, so this is a lookup and the caller decides what a
// hit means. Import-only — see Repository.FindActiveByName's doc comment.
//
// An archived class is deliberately not a hit: it still has deleted_at IS NULL,
// so a name-only match would enrol this year's students into a class the
// teacher closed last term.
func (s *Service) FindActiveByName(ctx context.Context, a authctx.Anchor, name string) (*Class, bool, error) {
	class, err := s.repo.FindActiveByName(ctx, a, name)
	if errors.Is(err, ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return class, true, nil
}

// ReassignTeacher moves the class and its schedule rows to newTeacherID within
// the scope. It is the classes-owned half of the owner-only class handoff: the
// coordinating handoff feature calls it inside a transaction that also moves
// the class's future planned sessions (owned by the sessions feature), so this
// touches only tables classes owns. ErrNotFound surfaces as a 404.
func (s *Service) ReassignTeacher(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) error {
	return translate(s.repo.ReassignTeacher(ctx, sc, classID, newTeacherID))
}

// ScheduleExists reports whether the class already carries this exact weekly
// slot. Import-only — see Repository.ScheduleExists's doc comment. Callers
// must pass the same effective_from they intend to write: AddScheduleAnchored
// defaults a blank one to the stored class's start_date, so a caller that
// checked with one date and wrote with another would append a duplicate slot
// on every run, and class_schedules has no unique index to stop it.
func (s *Service) ScheduleExists(ctx context.Context, a authctx.Anchor, classID uuid.UUID, weekday int16, startTime TimeOfDay, effectiveFrom time.Time) (bool, error) {
	return s.repo.ScheduleExists(ctx, a, classID, weekday, startTime, effectiveFrom)
}
