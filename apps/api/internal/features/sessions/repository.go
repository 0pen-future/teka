package sessions

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classscope"
)

// Row is a session joined with the class name the responses display, plus
// per-status attendance tallies over the session's live records. The counts
// are populated only by the read queries that go through
// withClassNameAndSummary; write-port fetches leave them zero, and the DTO
// layer reports them as null until attendance_confirmed_at is set — so a
// confirmed-but-empty session (zeros) stays distinguishable from a session
// never confirmed (null).
type Row struct {
	Session      `gorm:"embedded"`
	ClassName    string
	PresentCount int
	LateCount    int
	AbsentCount  int
	ExcusedCount int
}

// Repository is the persistence contract for sessions; the service depends on
// this interface, tests supply a fake.
type Repository interface {
	// BulkInsertIgnoreConflicts inserts rows, silently skipping any whose
	// (class_id, session_date) already exists among non-deleted rows — the
	// idempotency guarantee behind on-demand generation. Two concurrent
	// generations racing the same range both submit the same candidate rows;
	// Postgres, not application logic, decides which one lands.
	BulkInsertIgnoreConflicts(ctx context.Context, rows []Session) error
	// Create inserts a single ad-hoc session, translating the partial unique
	// index violation into ErrSessionExists so the caller sees a clean 409
	// instead of a silently dropped insert.
	Create(ctx context.Context, s *Session) error
	ListByClassAndRange(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Row, error)
	GetByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error)
	// ListByClassAndRangeReadable and GetReadableByID are the read port: own
	// sessions plus sessions of classes the caller holds a class_staff stint
	// on, ended stints included.
	ListByClassAndRangeReadable(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Row, error)
	GetReadableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error)
	// GetWritableByID is the WRITE port: the session is reachable only through
	// an ACTIVE class_staff stint on its class whose role is in roles (owner
	// bypasses via CenterWide). The service resolves roles from the capability
	// map and disambiguates 403 vs 404 through the read port.
	GetWritableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID, roles []string) (*Row, error)
	// UpdateStatus transitions a session's status and cancel_reason in one
	// statement; a nil cancelReason clears the column. Every current caller
	// supplies the full desired value, so there is no "leave untouched" case.
	// Like every write below, it reaches rows through the roles-bound write
	// scope, never through session ownership.
	UpdateStatus(ctx context.Context, sc authctx.Scope, roles []string, id uuid.UUID, status string, cancelReason *string) error
	SoftDelete(ctx context.Context, sc authctx.Scope, roles []string, id uuid.UUID) error
	// MarkHeldAndConfirmed transitions a session to held and stamps
	// attendance_confirmed_at in one statement — the transition attendance
	// confirmation performs implicitly, so confirming attendance never
	// requires a second button press.
	MarkHeldAndConfirmed(ctx context.Context, sc authctx.Scope, roles []string, id uuid.UUID, at time.Time) error
	// ListPending returns sessions strictly before `before` that are still
	// unconfirmed and planned or held (cancelled sessions never qualify),
	// optionally bounded by [from, to] (both inclusive), newest first. total
	// is the unlimited count; the returned rows respect limit. The expected
	// student count comes from one grouped join over enrollments, never a
	// per-row lookup — see pending.go.
	ListPending(ctx context.Context, sc authctx.Scope, before time.Time, from, to *time.Time, limit int) ([]PendingRow, int64, error)
	// ListPendingAnchored is ListPending's Anchor sibling for a caller-independent
	// gate — billing's period close runs it anchored on the period's own
	// teacher, never the acting caller's, so an owner closing a member's period
	// only ever blocks on that member's own unconfirmed sessions. Shares
	// ListPending's predicate and query builder; only the scoping differs.
	ListPendingAnchored(ctx context.Context, a authctx.Anchor, before time.Time, from, to *time.Time, limit int) ([]PendingRow, int64, error)
	// ReassignPlanned moves this class's future planned sessions to newTeacherID
	// on the context's transaction and returns how many moved. Only
	// status='planned' rows dated on or after notBefore move; held and cancelled
	// sessions and past planned sessions keep the old teacher, so attendance and
	// billing history never change. notBefore is "today" already resolved in the
	// teacher's timezone by the caller (see Service.ReassignPlanned) — the query
	// must not derive today from the DB clock, whose zone differs from the
	// teacher's. class_sessions is owned by sessions, so the class-handoff
	// feature moves them only through here.
	ReassignPlanned(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID, notBefore time.Time) (int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped returns the own-rows session query bound to one center: the owner
// reaches every session in their center, a member only the rows they teach
// themselves. It is the anchor-based write filter (handoff's ReassignPlanned)
// and the viewer-independent listing dashboard drives with a target scope, so
// a visibility key never widens it — readScoped is the port that widens.
// Composite FKs stop cross-center writes; only this filter stops cross-tenant
// reads. The center_id column is qualified because list queries join classes,
// which carries the same column name.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("class_sessions.center_id = ?", sc.CenterID)
	if !sc.WriteWide() {
		q = q.Where("class_sessions.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// readScoped is the READ port: own rows, sessions of classes the member holds
// a class_staff stint on (ended stints included), and every session in the
// center under sessions.view_all — the only place that key applies. Reads
// only: lifecycle writes (cancel, hold, delete) go through writeScoped's
// active-stint capability filter and handoff's ReassignPlanned through the
// own-rows scoped; a visibility key widens neither.
func (r *gormRepository) readScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("class_sessions.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermSessionsViewAll) {
		frag, _ := classscope.ReadExists("class_sessions.class_id")
		q = q.Where("(class_sessions.teacher_id = ? OR "+frag+")",
			sc.TeacherID, sc.TeacherID, sc.CenterID)
	}
	return q
}

// readScopedFeed is the pending-attendance feed's READ port: own rows,
// widened to the whole center under sessions.view_all. Unlike readScoped it
// deliberately carries no stint branch, because the same predicate backs
// billing's period-close gate, which runs it anchored on the period teacher
// — a stint that teacher holds on someone else's class must not pull that
// class's sessions into the blocking set.
func (r *gormRepository) readScopedFeed(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("class_sessions.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermSessionsViewAll) {
		q = q.Where("class_sessions.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// anchoredFeed is readScopedFeed's Anchor sibling: an unconditional
// teacher+center filter, no permission branch. Billing's period-close gate
// calls it anchored on the period's own teacher, never the acting caller's —
// a visibility key or a stint the anchor's teacher holds on someone else's
// class must never widen what the pending-attendance feed reports as
// blocking a close.
func (r *gormRepository) anchoredFeed(ctx context.Context, a authctx.Anchor) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Where("class_sessions.center_id = ? AND class_sessions.teacher_id = ?", a.CenterID, a.TeacherID)
}

// writeScoped is the capability write filter: a member reaches a session only
// through an ACTIVE stint on its class whose role is in roles — REPLACING the
// teacher_id filter, not OR-ing it, so an ended-stint teacher keeps history
// reads but loses every write, even on sessions still anchored to them. roles
// comes from the service's capability-map lookup; this method only binds it.
// Only the owner bypasses the stint filter (WriteWide): sessions.view_all is a
// visibility key and must never reach a write.
func (r *gormRepository) writeScoped(ctx context.Context, sc authctx.Scope, roles []string) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("class_sessions.center_id = ?", sc.CenterID)
	if !sc.WriteWide() {
		frag, _ := classscope.WriteExists("class_sessions.class_id")
		q = q.Where(frag, sc.TeacherID, sc.CenterID, roles)
	}
	return q
}

// withClassName joins the display name onto a session query. Matching on
// center_id (not teacher_id) keeps the composite-key discipline while still
// letting an owner's read of a member's session resolve the class name.
func withClassName(q *gorm.DB) *gorm.DB {
	return q.
		Joins("JOIN classes ON classes.id = class_sessions.class_id AND classes.center_id = class_sessions.center_id").
		Select("class_sessions.*, classes.name AS class_name")
}

// withClassNameAndSummary additionally aggregates the session's live
// attendance records into one count per status, riding the same grouped
// LEFT JOIN so a 60-session month listing stays a single round-trip. The
// status literals mirror the attendance_records CHECK constraint; only rows
// with deleted_at IS NULL count, so a student soft-removed from the roster
// after confirmation never inflates a tally.
func withClassNameAndSummary(q *gorm.DB) *gorm.DB {
	return q.
		Joins("JOIN classes ON classes.id = class_sessions.class_id AND classes.center_id = class_sessions.center_id").
		Joins(`LEFT JOIN attendance_records ON attendance_records.session_id = class_sessions.id
			AND attendance_records.center_id = class_sessions.center_id
			AND attendance_records.deleted_at IS NULL`).
		Select(`class_sessions.*, classes.name AS class_name,
			COUNT(attendance_records.id) FILTER (WHERE attendance_records.status = 'present')::int AS present_count,
			COUNT(attendance_records.id) FILTER (WHERE attendance_records.status = 'late')::int AS late_count,
			COUNT(attendance_records.id) FILTER (WHERE attendance_records.status = 'absent')::int AS absent_count,
			COUNT(attendance_records.id) FILTER (WHERE attendance_records.status = 'excused')::int AS excused_count`).
		Group("class_sessions.id, classes.id")
}

func (r *gormRepository) BulkInsertIgnoreConflicts(ctx context.Context, rows []Session) error {
	if len(rows) == 0 {
		return nil
	}
	// TargetWhere reproduces the partial index's predicate
	// (uq_class_sessions_per_day is WHERE deleted_at IS NULL) as the ON
	// CONFLICT target; without it Postgres cannot match the index and the
	// insert fails outright instead of skipping the conflicting rows.
	return database.FromContext(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "class_id"}, {Name: "session_date"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
			DoNothing:   true,
		}).
		Create(&rows).Error
}

func (r *gormRepository) Create(ctx context.Context, s *Session) error {
	err := database.FromContext(ctx, r.db).Create(s).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrSessionExists
	}
	return err
}

func (r *gormRepository) ListByClassAndRange(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Row, error) {
	var rows []Row
	err := withClassName(r.scoped(ctx, sc).Model(&Session{})).
		Where("class_sessions.class_id = ?", classID).
		Where("class_sessions.session_date BETWEEN ? AND ?", from, to).
		Order("class_sessions.session_date").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) GetByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	var row Row
	err := withClassName(r.scoped(ctx, sc).Model(&Session{})).
		Where("class_sessions.id = ?", id).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) ListByClassAndRangeReadable(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Row, error) {
	var rows []Row
	err := withClassNameAndSummary(r.readScoped(ctx, sc).Model(&Session{})).
		Where("class_sessions.class_id = ?", classID).
		Where("class_sessions.session_date BETWEEN ? AND ?", from, to).
		Order("class_sessions.session_date").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) GetReadableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	var row Row
	err := withClassNameAndSummary(r.readScoped(ctx, sc).Model(&Session{})).
		Where("class_sessions.id = ?", id).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) GetWritableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID, roles []string) (*Row, error) {
	var row Row
	err := withClassName(r.writeScoped(ctx, sc, roles).Model(&Session{})).
		Where("class_sessions.id = ?", id).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) UpdateStatus(ctx context.Context, sc authctx.Scope, roles []string, id uuid.UUID, status string, cancelReason *string) error {
	res := r.writeScoped(ctx, sc, roles).
		Model(&Session{}).
		Where("class_sessions.id = ?", id).
		Updates(map[string]any{"status": status, "cancel_reason": cancelReason})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SoftDelete(ctx context.Context, sc authctx.Scope, roles []string, id uuid.UUID) error {
	res := r.writeScoped(ctx, sc, roles).Where("class_sessions.id = ?", id).Delete(&Session{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ReassignPlanned moves the class's future planned sessions to newTeacherID and
// returns the count moved. The date predicate is inclusive of today
// (session_date >= notBefore): a late-day handoff carries today's still-planned
// session; the owner records attendance first if it already ran, and a held
// session never matches this filter. notBefore is today resolved in the
// teacher's timezone by the caller — never CURRENT_DATE, whose DB-session zone
// (UTC in deployment) would count a session dated yesterday-VN as "today" in the
// early-morning window and wrongly sweep a past planned session. Past planned,
// held, and cancelled sessions are left untouched so history and closed books
// stay with the old teacher. Runs on the context's transaction for atomicity
// with the class move.
func (r *gormRepository) ReassignPlanned(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID, notBefore time.Time) (int64, error) {
	res := r.scoped(ctx, sc).
		Model(&Session{}).
		Where("class_sessions.class_id = ? AND class_sessions.status = ?", classID, StatusPlanned).
		Where("class_sessions.session_date >= ?", notBefore).
		Update("teacher_id", newTeacherID)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (r *gormRepository) MarkHeldAndConfirmed(ctx context.Context, sc authctx.Scope, roles []string, id uuid.UUID, at time.Time) error {
	res := r.writeScoped(ctx, sc, roles).
		Model(&Session{}).
		Where("class_sessions.id = ?", id).
		Updates(map[string]any{"status": StatusHeld, "attendance_confirmed_at": at})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
