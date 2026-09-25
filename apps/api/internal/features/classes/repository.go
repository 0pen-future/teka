package classes

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classscope"
	"teka/apps/api/internal/shared/likeq"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows the class list. Status is one of StatusActive,
// StatusArchived, or "" for every non-deleted class regardless of status.
// Every other field is optional and they combine with AND: Phase is one of
// the Phase* constants judged against today; Q matches name or code
// case-insensitively as a substring; Weekday and Shift look for a timetable
// row still effective today (Shift is one of the Shift* constants); Tag
// matches one tag element exactly; Recruiting keeps only the classes open for
// recruitment (see OpenForRecruitment).
type ListFilter struct {
	Status  string
	Phase   string
	Q       string
	Weekday *int16
	Shift   string
	Tag     string
	// CourseID narrows to the classes attached to one course.
	CourseID   *uuid.UUID
	Recruiting bool
}

// AvailabilitySchedule is one schedule row's weekday/time/duration, the only
// fields the availability endpoint's overlap check needs.
type AvailabilitySchedule struct {
	Weekday     int16
	StartTime   TimeOfDay
	DurationMin int16
}

// AvailabilityClassRow is one live class's room, teacher, active schedules
// (effective today or later) and active class_staff member ids — the raw
// material the availability endpoint's busy computation runs over.
type AvailabilityClassRow struct {
	ClassID   uuid.UUID
	Room      string
	TeacherID uuid.UUID
	Schedules []AvailabilitySchedule
	StaffIDs  []uuid.UUID
}

// DirectoryMember is one active center member, as listed by the
// availability endpoint's teachers array.
type DirectoryMember struct {
	TeacherID uuid.UUID
	Name      string
}

// Repository is the persistence contract for classes and their schedules; the
// service depends on this interface, tests supply a fake.
type Repository interface {
	// CreateWithSchedules inserts the class and its schedule rows on the
	// context's transaction; the caller wraps it in WithinTx for atomicity.
	CreateWithSchedules(ctx context.Context, class *Class, schedules []Schedule) error
	GetByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Class, error)
	List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, int64, error)
	// LinkNextClass sets classID as the parent of childID (nil unlinks) and
	// unlinks every other live class currently pointing at classID, so a
	// class keeps at most one next-class link (lớp kế tiếp). Runs on the
	// context's transaction: the caller wraps it in the same WithinTx block
	// as the rest of Update so both writes commit or roll back together.
	// ErrNotFound when childID is set but does not resolve to a live class of
	// the anchor's center.
	LinkNextClass(ctx context.Context, a authctx.Anchor, classID uuid.UUID, childID *uuid.UUID) error
	// AvailabilityClasses returns every live class of the center (excluding
	// excludeClassID when set) with its room, teacher and the schedule/staff
	// facts GET /classes/availability's busy computation needs.
	AvailabilityClasses(ctx context.Context, sc authctx.Scope, excludeClassID *uuid.UUID) ([]AvailabilityClassRow, error)
	// ActiveMemberDirectory lists the center's active members for the
	// availability endpoint's teacher list — duplicates centers.Directory's
	// join because centers imports classes and a reverse import would cycle.
	ActiveMemberDirectory(ctx context.Context, sc authctx.Scope) ([]DirectoryMember, error)
	// GetReadableByID and ListReadable are the READ port: own rows plus any
	// class the caller holds a class_staff stint on (ended included — history
	// reads). GetByID/List stay own-rows because they double as the write
	// gate for teaching, sessions, grading, and the dashboard.
	GetReadableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Class, error)
	ListReadable(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, int64, error)
	// GetWritableByID is the WRITE port: the class is reachable only through
	// an ACTIVE class_staff stint whose role is in roles (owner bypasses via
	// CenterWide). roles is the service's capability-map lookup — this method
	// only binds it. ErrNotFound both when the class does not exist and when
	// the caller merely lacks the capability; the service disambiguates 403
	// vs 404 through the read port.
	GetWritableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID, roles []string) (*Class, error)
	Update(ctx context.Context, class *Class) error
	Archive(ctx context.Context, sc authctx.Scope, id uuid.UUID) error
	SoftDelete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error
	CountOpenEnrollments(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (int64, error)
	// CountActiveEnrollmentsByClass is CountOpenEnrollments over a page of
	// class ids in one grouped query, keyed by class id (absent key = 0),
	// narrowed to the enrollments the caller could list.
	CountActiveEnrollmentsByClass(ctx context.Context, sc authctx.Scope, classIDs []uuid.UUID) (map[uuid.UUID]int64, error)
	// CountReadableByPhase buckets every class the caller can read (the same
	// port ListReadable uses) by its phase on today, in one query.
	// recruitingOnly narrows the counted rows to those open for recruitment.
	CountReadableByPhase(ctx context.Context, sc authctx.Scope, today time.Time, recruitingOnly bool) (ClassStatsResponse, error)
	// LiveClassInCenter reports whether a live (non-deleted) class with this
	// id exists in the anchor's center — the parent-class validation.
	LiveClassInCenter(ctx context.Context, a authctx.Anchor, classID uuid.UUID) (bool, error)
	// ParentCreatesCycle reports whether selfID appears anywhere in
	// parentID's own ancestor chain (parentID, its parent, its parent's
	// parent, …) within the anchor's center. If it does, pointing selfID at
	// parentID would close a loop in the lineage. The walk is depth-capped so
	// a corrupted chain cannot loop the query itself.
	ParentCreatesCycle(ctx context.Context, a authctx.Anchor, parentID, selfID uuid.UUID) (bool, error)
	// CodeExists reports whether a live class in the anchor's center other
	// than exceptID already carries code — the partial unique index's
	// predicate, checked ahead of the insert so the caller gets a 409 rather
	// than a constraint error. Center-wide on purpose: the index is per
	// center, not per teacher, so a member's probe must see the owner's rows.
	CodeExists(ctx context.Context, a authctx.Anchor, code string, exceptID uuid.UUID) (bool, error)
	// FindCourse resolves a live course of the anchor's center, whatever
	// its status: an archived course still anchors the classes it already
	// has. ErrCourseNotFound when missing or of another center.
	FindCourse(ctx context.Context, a authctx.Anchor, courseID uuid.UUID) (*CourseRef, error)

	AddSchedule(ctx context.Context, s *Schedule) error
	GetSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) (*Schedule, error)
	UpdateSchedule(ctx context.Context, s *Schedule) error
	SoftDeleteSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) error
	// ListEffectiveSchedules returns the class's schedule rows whose
	// [effective_from, effective_to] range intersects [from, to], treating a
	// NULL effective_to as open-ended. This is the contract session
	// generation (plan 03) consumes.
	ListEffectiveSchedules(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Schedule, error)
	// FindActiveByName resolves a live, active class by its exact name under
	// one anchor. Import-only: the roster import already resolved which
	// teacher a class row belongs to (the workbook names it directly), so
	// this takes an Anchor rather than a Scope — there is no caller-scoped
	// route handler for it to serve. There is no unique index on
	// classes.name, so this is a lookup, not a constraint.
	FindActiveByName(ctx context.Context, a authctx.Anchor, name string) (*Class, error)
	// ScheduleExists reports whether the class already carries this exact
	// weekly slot, including its effective_from. Import-only, like
	// FindActiveByName — see its doc comment.
	ScheduleExists(ctx context.Context, a authctx.Anchor, classID uuid.UUID, weekday int16, startTime TimeOfDay, effectiveFrom time.Time) (bool, error)
	// ReassignTeacher moves the class and every one of its schedule rows to
	// newTeacherID within the scope, on the context's transaction. Both tables
	// are owned by classes, so the class-handoff feature moves them only
	// through here. ErrNotFound when the class is not visible in the scope.
	ReassignTeacher(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped returns the own-rows classes query bound to one center: the owner
// reaches every class in their center, a member only the rows anchored to
// them. It doubles as the settings write gate (Update, Archive, SoftDelete,
// ReassignTeacher) and as the own-rows lookup other features pass an anchor
// scope to, so a visibility key never widens it — readScoped is the port that
// widens. Composite FKs stop cross-center writes; only this filter stops
// cross-tenant reads.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("classes.center_id = ?", sc.CenterID)
	if !sc.WriteWide() {
		q = q.Where("classes.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// anchored is the unconditional two-column filter behind FindActiveByName and
// ScheduleExists: the roster import already resolved which teacher a class
// row belongs to, so no WriteWide branch applies — unlike scoped, an Anchor
// carries no owner bypass to check.
func (r *gormRepository) anchored(ctx context.Context, a authctx.Anchor) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Where("classes.center_id = ? AND classes.teacher_id = ?", a.CenterID, a.TeacherID)
}

// readScoped is the stint read filter: a member sees exactly the classes they
// hold any class_staff stint on — ended stints included, so a closed
// assignment keeps read access to the class's history. There is no creator
// (teacher_id) arm: every class's current teacher holds a giao_vien stint —
// ACTIVE via the create-hook and handoff dual-write, or soft-closed when the
// holder left the center — so the pointer can never reach a row the stint
// filter misses. That is also why ReadExists deliberately ignores ended_at:
// filtering to ACTIVE would strand departed-and-returned teachers off their
// own classes. classes.view_all widens this port and only this port: write
// paths resolve through scoped or writeScoped, which a visibility key never
// widens.
func (r *gormRepository) readScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("classes.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermClassesViewAll) {
		frag, _ := classscope.ReadExists("classes.id")
		q = q.Where(frag, sc.TeacherID, sc.CenterID)
	}
	return q
}

// writeScoped is the capability write filter: a member reaches a class only
// through an ACTIVE class_staff stint whose role is in roles — REPLACING the
// creator (teacher_id) filter, not OR-ing it, so a teacher whose stint ended
// (handoff) loses writes even on rows still anchored to them. roles comes from
// the service's capability-map lookup; this method only binds it. Only the
// owner bypasses the stint filter (WriteWide): classes.view_all is a
// visibility key and must never reach a write.
func (r *gormRepository) writeScoped(ctx context.Context, sc authctx.Scope, roles []string) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("classes.center_id = ?", sc.CenterID)
	if !sc.WriteWide() {
		frag, _ := classscope.WriteExists("classes.id")
		q = q.Where(frag, sc.TeacherID, sc.CenterID, roles)
	}
	return q
}

// readScopedSchedules is the READ port for the class_schedules table: own
// rows, plus every row in the center under classes.view_all. Session
// generation consumes it, so a viewer allowed to materialise a class's
// sessions sees the same timetable the class's teacher does.
func (r *gormRepository) readScopedSchedules(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("class_schedules.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermClassesViewAll) {
		q = q.Where("class_schedules.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// writeScopedSchedules is scoped's counterpart for the class_schedules table:
// the owner reaches every row, a member only the rows anchored to them. A
// visibility key never widens it.
// readScopedEnrollments mirrors the enrollments feature's read port — own
// rows, rows on any class the caller holds a class_staff stint on, and the
// whole center under enrollments.view_all — so a headcount surfaced from this
// feature never exceeds what the roster list would show the same caller.
func (r *gormRepository) readScopedEnrollments(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Table("enrollments").
		Where("enrollments.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermEnrollmentsViewAll) {
		frag, _ := classscope.ReadExists("enrollments.class_id")
		q = q.Where("(enrollments.teacher_id = ? OR "+frag+")",
			sc.TeacherID, sc.TeacherID, sc.CenterID)
	}
	return q
}

func (r *gormRepository) writeScopedSchedules(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("class_schedules.center_id = ?", sc.CenterID)
	if !sc.WriteWide() {
		q = q.Where("class_schedules.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// anchoredSchedules is anchored's class_schedules counterpart, behind
// ScheduleExists: the unconditional two-column filter for the roster import,
// which already resolved the class's own teacher.
func (r *gormRepository) anchoredSchedules(ctx context.Context, a authctx.Anchor) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Where("class_schedules.center_id = ? AND class_schedules.teacher_id = ?", a.CenterID, a.TeacherID)
}

// preloadSchedules orders schedule rows deterministically for display.
func preloadSchedules(db *gorm.DB) *gorm.DB {
	return db.Order("effective_from, weekday, start_time")
}

func (r *gormRepository) CreateWithSchedules(ctx context.Context, class *Class, schedules []Schedule) error {
	db := database.FromContext(ctx, r.db)
	// Omit the associations: schedule rows are inserted explicitly below with
	// ids and teacher ids already set, and the course is a reference the
	// classes feature never writes.
	if err := db.Omit(clause.Associations).Create(class).Error; err != nil {
		return err
	}
	if len(schedules) == 0 {
		return nil
	}
	return db.Create(&schedules).Error
}

func (r *gormRepository) GetByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Class, error) {
	var class Class
	err := r.scoped(ctx, sc).
		Preload("Schedules", preloadSchedules).Preload("Course", "deleted_at IS NULL").
		Take(&class, "classes.id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if class.NextClassID, err = r.nextChildID(ctx, class.ID); err != nil {
		return nil, err
	}
	return &class, nil
}

func (r *gormRepository) GetWritableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID, roles []string) (*Class, error) {
	var class Class
	err := r.writeScoped(ctx, sc, roles).
		Preload("Schedules", preloadSchedules).Preload("Course", "deleted_at IS NULL").
		Take(&class, "classes.id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if class.NextClassID, err = r.nextChildID(ctx, class.ID); err != nil {
		return nil, err
	}
	return &class, nil
}

func (r *gormRepository) GetReadableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Class, error) {
	var class Class
	err := r.readScoped(ctx, sc).
		Preload("Schedules", preloadSchedules).Preload("Course", "deleted_at IS NULL").
		Take(&class, "classes.id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if class.NextClassID, err = r.nextChildID(ctx, class.ID); err != nil {
		return nil, err
	}
	return &class, nil
}

// nextChildID resolves the earliest-created live child of classID (the
// earliest classes.created_at, ties broken by id, among live rows whose
// parent_class_id is classID), or nil when it has none. "Live" matches
// LiveClassInCenter: deleted_at IS NULL, any status.
func (r *gormRepository) nextChildID(ctx context.Context, classID uuid.UUID) (*uuid.UUID, error) {
	var row struct{ NextID *uuid.UUID }
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT (
			SELECT id FROM classes
			WHERE parent_class_id = ? AND deleted_at IS NULL
			ORDER BY created_at, id
			LIMIT 1
		) AS next_id`, classID).
		Scan(&row).Error
	return row.NextID, err
}

// nextChildIDs is nextChildID batched over a page of ids in one query, keyed
// by parent id (absent key = no live child).
func (r *gormRepository) nextChildIDs(ctx context.Context, classIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	result := make(map[uuid.UUID]uuid.UUID, len(classIDs))
	if len(classIDs) == 0 {
		return result, nil
	}
	var rows []struct {
		ParentClassID uuid.UUID
		ID            uuid.UUID
	}
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT DISTINCT ON (parent_class_id) parent_class_id, id
			FROM classes
			WHERE parent_class_id IN ? AND deleted_at IS NULL
			ORDER BY parent_class_id, created_at, id`, classIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ParentClassID] = row.ID
	}
	return result, nil
}

func (r *gormRepository) ListReadable(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, int64, error) {
	return r.list(ctx, r.readScoped(ctx, sc), filter, p)
}

func (r *gormRepository) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, int64, error) {
	return r.list(ctx, r.scoped(ctx, sc), filter, p)
}

func (r *gormRepository) list(ctx context.Context, q *gorm.DB, filter ListFilter, p pagination.Params) ([]Class, int64, error) {
	q = q.Model(&Class{})
	if filter.Status != "" {
		// The default active-only list matches the idx_classes_teacher
		// partial-index predicate (deleted_at IS NULL AND status = 'active').
		q = q.Where("classes.status = ?", filter.Status)
	}
	if filter.Q != "" {
		needle := likeq.Contains(filter.Q)
		q = q.Where(`(classes.name ILIKE ? ESCAPE '\' OR classes.code ILIKE ? ESCAPE '\')`, needle, needle)
	}
	if filter.Tag != "" {
		// Bound as a JSON array so the containment check matches one whole
		// element; a raw string could never be built into the predicate.
		tag, err := json.Marshal([]string{filter.Tag})
		if err != nil {
			return nil, 0, err
		}
		q = q.Where("classes.tags @> ?::jsonb", string(tag))
	}
	if filter.CourseID != nil {
		q = q.Where("classes.course_id = ?", *filter.CourseID)
	}
	today := Today()
	if filter.Weekday != nil || filter.Shift != "" {
		slot := `EXISTS (SELECT 1 FROM class_schedules s
			WHERE s.class_id = classes.id AND s.deleted_at IS NULL
			  AND (s.effective_to IS NULL OR s.effective_to >= ?)`
		args := []any{today}
		if filter.Weekday != nil {
			slot += " AND s.weekday = ?"
			args = append(args, *filter.Weekday)
		}
		if from, to, ok := ShiftBounds(filter.Shift); ok {
			slot += " AND s.start_time >= ?"
			args = append(args, from)
			if to != "" {
				slot += " AND s.start_time < ?"
				args = append(args, to)
			}
		}
		q = q.Where(slot+")", args...)
	}
	if frag, args, ok := PhasePredicate(filter.Phase, today); ok {
		q = q.Where(frag, args...)
	}
	if filter.Recruiting {
		frag, args := RecruitingPredicate(today)
		q = q.Where(frag, args...)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Class
	err := q.Preload("Schedules", preloadSchedules).Preload("Course", "deleted_at IS NULL").Scopes(p.Scope).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	nextIDs, err := r.nextChildIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range rows {
		if childID, ok := nextIDs[rows[i].ID]; ok {
			id := childID
			rows[i].NextClassID = &id
		}
	}
	return rows, total, nil
}

func (r *gormRepository) Update(ctx context.Context, class *Class) error {
	return database.FromContext(ctx, r.db).Omit(clause.Associations).Save(class).Error
}

func (r *gormRepository) LiveClassInCenter(ctx context.Context, a authctx.Anchor, classID uuid.UUID) (bool, error) {
	var exists bool
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT EXISTS (
			SELECT 1 FROM classes
			WHERE id = ? AND center_id = ? AND deleted_at IS NULL)`, classID, a.CenterID).
		Scan(&exists).Error
	return exists, err
}

// maxLineageDepth caps the ancestor walk well past any plausible split
// chain, so a corrupted or (impossibly, given this same guard) cyclic chain
// cannot turn the recursive query itself into an infinite loop.
const maxLineageDepth = 50

func (r *gormRepository) ParentCreatesCycle(ctx context.Context, a authctx.Anchor, parentID, selfID uuid.UUID) (bool, error) {
	var found bool
	err := database.FromContext(ctx, r.db).
		Raw(`WITH RECURSIVE ancestors AS (
			SELECT id, parent_class_id, 1 AS depth
			FROM classes
			WHERE id = ? AND center_id = ? AND deleted_at IS NULL
			UNION ALL
			SELECT c.id, c.parent_class_id, ancestors.depth + 1
			FROM classes c
			JOIN ancestors ON c.id = ancestors.parent_class_id
			WHERE c.center_id = ? AND c.deleted_at IS NULL AND ancestors.depth < ?
		)
		SELECT EXISTS (SELECT 1 FROM ancestors WHERE id = ?)`,
			parentID, a.CenterID, a.CenterID, maxLineageDepth, selfID).
		Scan(&found).Error
	return found, err
}

func (r *gormRepository) LinkNextClass(ctx context.Context, a authctx.Anchor, classID uuid.UUID, childID *uuid.UUID) error {
	db := database.FromContext(ctx, r.db)
	unlink := db.Model(&Class{}).
		Where("classes.center_id = ? AND classes.parent_class_id = ? AND classes.deleted_at IS NULL", a.CenterID, classID)
	if childID != nil {
		unlink = unlink.Where("classes.id <> ?", *childID)
	}
	if err := unlink.Update("parent_class_id", nil).Error; err != nil {
		return err
	}
	if childID == nil {
		return nil
	}
	res := db.Model(&Class{}).
		Where("classes.id = ? AND classes.center_id = ? AND classes.deleted_at IS NULL", *childID, a.CenterID).
		Update("parent_class_id", classID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// teacherAccountStatusActive mirrors teachers.StatusActive. It is duplicated
// as a literal, rather than imported, because ActiveMemberDirectory below
// already duplicates centers.Directory's join for the import-cycle reason
// documented there — importing the teachers package for this one constant
// would add a dependency edge this package does not otherwise need.
const teacherAccountStatusActive = "active"

// AvailabilityClasses fetches every live class of the center (deleted_at IS
// NULL, any status — the LiveClassInCenter definition), excluding
// excludeClassID when set, together with its active schedules (effective
// today or later) and active class_staff member ids. Three queries batched
// by class id, rather than one join, keep the row shape flat and avoid a
// fan-out multiplying class rows by schedule/staff rows.
func (r *gormRepository) AvailabilityClasses(ctx context.Context, sc authctx.Scope, excludeClassID *uuid.UUID) ([]AvailabilityClassRow, error) {
	db := database.FromContext(ctx, r.db)
	var classRows []struct {
		ID        uuid.UUID
		Room      string
		TeacherID uuid.UUID
	}
	q := db.Table("classes").
		Select("id, room, teacher_id").
		Where("center_id = ? AND deleted_at IS NULL", sc.CenterID)
	if excludeClassID != nil {
		q = q.Where("id <> ?", *excludeClassID)
	}
	if err := q.Scan(&classRows).Error; err != nil {
		return nil, err
	}
	if len(classRows) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(classRows))
	for i, c := range classRows {
		ids[i] = c.ID
	}

	today := Today()
	var scheduleRows []struct {
		ClassID     uuid.UUID
		Weekday     int16
		StartTime   TimeOfDay
		DurationMin int16
	}
	if err := db.Table("class_schedules").
		Select("class_id, weekday, start_time, duration_min").
		Where("class_id IN ? AND deleted_at IS NULL AND (effective_to IS NULL OR effective_to >= ?)", ids, today).
		Scan(&scheduleRows).Error; err != nil {
		return nil, err
	}
	schedulesByClass := make(map[uuid.UUID][]AvailabilitySchedule, len(classRows))
	for _, s := range scheduleRows {
		schedulesByClass[s.ClassID] = append(schedulesByClass[s.ClassID],
			AvailabilitySchedule{Weekday: s.Weekday, StartTime: s.StartTime, DurationMin: s.DurationMin})
	}

	var staffRows []struct {
		ClassID   uuid.UUID
		TeacherID uuid.UUID
	}
	if err := db.Table("class_staff").
		Select("class_id, teacher_id").
		Where("class_id IN ? AND ended_at IS NULL", ids).
		Scan(&staffRows).Error; err != nil {
		return nil, err
	}
	staffByClass := make(map[uuid.UUID][]uuid.UUID, len(classRows))
	for _, s := range staffRows {
		staffByClass[s.ClassID] = append(staffByClass[s.ClassID], s.TeacherID)
	}

	out := make([]AvailabilityClassRow, len(classRows))
	for i, c := range classRows {
		out[i] = AvailabilityClassRow{
			ClassID:   c.ID,
			Room:      c.Room,
			TeacherID: c.TeacherID,
			Schedules: schedulesByClass[c.ID],
			StaffIDs:  staffByClass[c.ID],
		}
	}
	return out, nil
}

// ActiveMemberDirectory duplicates centers.Directory's join (center_members
// + teachers + user_accounts) rather than importing the centers package,
// because centers already imports classes (for its dashboard) and a reverse
// import would cycle.
func (r *gormRepository) ActiveMemberDirectory(ctx context.Context, sc authctx.Scope) ([]DirectoryMember, error) {
	var rows []DirectoryMember
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT t.id AS teacher_id, t.full_name AS name
		FROM center_members cm
		JOIN teachers t ON t.id = cm.teacher_id AND t.deleted_at IS NULL
		JOIN user_accounts ua ON ua.id = t.id AND ua.deleted_at IS NULL AND ua.status = ?
		WHERE cm.center_id = ? AND cm.left_at IS NULL
		ORDER BY t.full_name, t.id`,
		teacherAccountStatusActive, sc.CenterID).Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) FindCourse(ctx context.Context, a authctx.Anchor, courseID uuid.UUID) (*CourseRef, error) {
	var ref CourseRef
	// FOR SHARE: a course delete locks the row FOR UPDATE, so an attach that
	// runs inside the class's write transaction either waits for the delete
	// (and then misses the row) or holds the delete off until it commits.
	err := database.FromContext(ctx, r.db).
		Clauses(clause.Locking{Strength: "SHARE"}).
		Where("courses.center_id = ? AND courses.deleted_at IS NULL", a.CenterID).
		Take(&ref, "courses.id = ?", courseID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCourseNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

func (r *gormRepository) Archive(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.scoped(ctx, sc).
		Model(&Class{}).
		Where("classes.id = ?", id).
		Update("status", StatusArchived)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SoftDelete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.scoped(ctx, sc).Where("classes.id = ?", id).Delete(&Class{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountOpenEnrollments is deliberately center-wide: it guards class deletion
// against open enrollments, an integrity fact about the class that does not
// depend on who is asking. Narrowing it to the caller's rows would let a
// delete slip past enrollments anchored by someone else.
func (r *gormRepository) CountOpenEnrollments(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (int64, error) {
	var n int64
	err := database.FromContext(ctx, r.db).
		Table("enrollments").
		Where("center_id = ? AND class_id = ? AND ended_on IS NULL AND deleted_at IS NULL", sc.CenterID, classID).
		Count(&n).Error
	return n, err
}

// CountActiveEnrollmentsByClass applies CountOpenEnrollments' predicate
// (ended_on IS NULL, not deleted — what GET /enrollments?active=true lists)
// across every id at once so a class page never counts per row. The count
// is returned to the client, so unlike CountOpenEnrollments it follows the
// enrollments read filter: a member without enrollments.view_all only counts
// rows they own or hold a class_staff stint on, and never learns a headcount
// they could not list.
func (r *gormRepository) CountActiveEnrollmentsByClass(ctx context.Context, sc authctx.Scope, classIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	counts := make(map[uuid.UUID]int64, len(classIDs))
	if len(classIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		ClassID uuid.UUID
		N       int64
	}
	err := r.readScopedEnrollments(ctx, sc).
		Select("enrollments.class_id, COUNT(*) AS n").
		Where("enrollments.class_id IN ? AND enrollments.ended_on IS NULL AND enrollments.deleted_at IS NULL", classIDs).
		Group("enrollments.class_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ClassID] = row.N
	}
	return counts, nil
}

// CountReadableByPhase runs the four phase predicates as SUM(CASE ...) arms
// over readScoped — the one place classes.view_all may widen a read — so the
// chips on the list page count exactly the rows the list would show.
func (r *gormRepository) CountReadableByPhase(ctx context.Context, sc authctx.Scope, today time.Time, recruitingOnly bool) (ClassStatsResponse, error) {
	recruitingFrag, recruitingArgs := RecruitingPredicate(today)
	selectSQL := "COUNT(*) AS all_count, SUM(CASE WHEN " + recruitingFrag + " THEN 1 ELSE 0 END) AS recruiting"
	args := append([]any{}, recruitingArgs...)
	for _, phase := range []string{PhaseUpcoming, PhaseRunning, PhaseEnded, PhaseArchived} {
		frag, phaseArgs, _ := PhasePredicate(phase, today)
		selectSQL += ", SUM(CASE WHEN " + frag + " THEN 1 ELSE 0 END) AS " + phase
		args = append(args, phaseArgs...)
	}
	var row struct {
		AllCount   int64
		Upcoming   int64
		Running    int64
		Ended      int64
		Archived   int64
		Recruiting int64
	}
	q := r.readScoped(ctx, sc).Model(&Class{})
	if recruitingOnly {
		q = q.Where(recruitingFrag, recruitingArgs...)
	}
	err := q.Select(selectSQL, args...).Scan(&row).Error
	if err != nil {
		return ClassStatsResponse{}, err
	}
	return ClassStatsResponse{
		All:        row.AllCount,
		Upcoming:   row.Upcoming,
		Running:    row.Running,
		Ended:      row.Ended,
		Archived:   row.Archived,
		Recruiting: row.Recruiting,
	}, nil
}

// CodeExists probes the partial unique index's predicate: live rows in the
// center, minus the row being edited. Not routed through scoped: the index
// spans the whole center, so a member must be told about the owner's code
// too, or their insert would fail on the constraint instead.
func (r *gormRepository) CodeExists(ctx context.Context, a authctx.Anchor, code string, exceptID uuid.UUID) (bool, error) {
	var n int64
	err := database.FromContext(ctx, r.db).Model(&Class{}).
		Where("classes.center_id = ? AND classes.code = ? AND classes.id <> ?", a.CenterID, code, exceptID).
		Count(&n).Error
	return n > 0, err
}

func (r *gormRepository) AddSchedule(ctx context.Context, s *Schedule) error {
	return database.FromContext(ctx, r.db).Create(s).Error
}

func (r *gormRepository) GetSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) (*Schedule, error) {
	var s Schedule
	err := r.writeScopedSchedules(ctx, sc).
		Take(&s, "class_schedules.id = ? AND class_schedules.class_id = ?", scheduleID, classID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrScheduleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *gormRepository) UpdateSchedule(ctx context.Context, s *Schedule) error {
	return database.FromContext(ctx, r.db).Save(s).Error
}

func (r *gormRepository) SoftDeleteSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) error {
	res := r.writeScopedSchedules(ctx, sc).
		Where("class_schedules.id = ? AND class_schedules.class_id = ?", scheduleID, classID).
		Delete(&Schedule{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

func (r *gormRepository) ListEffectiveSchedules(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Schedule, error) {
	var rows []Schedule
	err := r.readScopedSchedules(ctx, sc).
		Where("class_schedules.class_id = ?", classID).
		Where("class_schedules.effective_from <= ? AND (class_schedules.effective_to IS NULL OR class_schedules.effective_to >= ?)", to, from).
		Order("effective_from, weekday, start_time").
		Find(&rows).Error
	return rows, err
}

// FindActiveByName resolves a class by exact name under one anchor. status is
// part of the match: an archived class still has deleted_at IS NULL, so a
// name-only lookup would hand a bulk importer a class the teacher closed last
// term and quietly enrol this year's students into it.
func (r *gormRepository) FindActiveByName(ctx context.Context, a authctx.Anchor, name string) (*Class, error) {
	var class Class
	err := r.anchored(ctx, a).
		Where("classes.name = ? AND classes.status = ?", name, StatusActive).
		Take(&class).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &class, nil
}

// ReassignTeacher stamps newTeacherID onto the class and all its live schedule
// rows. Both statements run on the context's transaction so a class-handoff
// coordinator can move them atomically with the class's future sessions. The
// class update must touch a row (else the class is not in scope → ErrNotFound);
// the schedule update may touch none — a class can carry no live slot — so its
// RowsAffected is not asserted.
func (r *gormRepository) ReassignTeacher(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) error {
	res := r.scoped(ctx, sc).
		Model(&Class{}).
		Where("classes.id = ?", classID).
		Update("teacher_id", newTeacherID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return r.writeScopedSchedules(ctx, sc).
		Model(&Schedule{}).
		Where("class_schedules.class_id = ?", classID).
		Update("teacher_id", newTeacherID).Error
}

// ScheduleExists reports whether an identical live slot is already on the
// class. effective_from is part of the identity because the same weekday and
// time can legitimately recur after a timetable change closes the old row.
func (r *gormRepository) ScheduleExists(ctx context.Context, a authctx.Anchor, classID uuid.UUID, weekday int16, startTime TimeOfDay, effectiveFrom time.Time) (bool, error) {
	var count int64
	err := r.anchoredSchedules(ctx, a).Model(&Schedule{}).
		Where("class_schedules.class_id = ? AND class_schedules.weekday = ?", classID, weekday).
		Where("class_schedules.start_time = ? AND class_schedules.effective_from = ?", startTime, effectiveFrom).
		Count(&count).Error
	return count > 0, err
}
