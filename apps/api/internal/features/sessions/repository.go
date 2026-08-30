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

// Row is a session joined with the class name the responses display.
type Row struct {
	Session   `gorm:"embedded"`
	ClassName string
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

// scoped returns a session query bound to one center. An owner sees every
// session in their center; a member sees only the rows they teach themselves.
// Composite FKs stop cross-center writes; only this filter stops cross-tenant
// reads. The center_id column is qualified because list queries join classes,
// which carries the same column name.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("class_sessions.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermSessionsViewAll) {
		q = q.Where("class_sessions.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// readScoped additionally lets a member read sessions of classes they hold a
// class_staff stint on, ended stints included. Reads only: lifecycle writes
// (cancel, hold, delete) go through writeScoped's active-stint capability
// filter; handoff's ReassignPlanned keeps scoped because it runs under the
// owner-driven handoff flow.
func (r *gormRepository) readScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("class_sessions.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermSessionsViewAll) {
		frag, _ := classscope.ReadExists("class_sessions.class_id")
		q = q.Where("(class_sessions.teacher_id = ? OR "+frag+")",
			sc.TeacherID, sc.TeacherID, sc.CenterID)
	}
	return q
}

// writeScoped is the capability write filter: a member reaches a session only
// through an ACTIVE stint on its class whose role is in roles — REPLACING the
// teacher_id filter, not OR-ing it, so an ended-stint teacher keeps history
// reads but loses every write, even on sessions still anchored to them. roles
// comes from the service's capability-map lookup; this method only binds it.
func (r *gormRepository) writeScoped(ctx context.Context, sc authctx.Scope, roles []string) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("class_sessions.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermSessionsViewAll) {
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
	err := withClassName(r.readScoped(ctx, sc).Model(&Session{})).
		Where("class_sessions.class_id = ?", classID).
		Where("class_sessions.session_date BETWEEN ? AND ?", from, to).
		Order("class_sessions.session_date").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) GetReadableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	var row Row
	err := withClassName(r.readScoped(ctx, sc).Model(&Session{})).
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
