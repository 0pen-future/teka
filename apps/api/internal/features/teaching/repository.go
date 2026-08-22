package teaching

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
)

// QueueRow is one pending giáo án in the owner's review queue, already
// joined to its display context. LessonTitle and TeacherName are pointers
// because both joins are NULL-safe: the curriculum may have shrunk below the
// plan's index (or never existed), and a pending plan submitted before the
// submitter column existed carries no name.
type QueueRow struct {
	PlanID      uuid.UUID
	ClassID     uuid.UUID
	ClassName   string
	LessonIndex int
	LessonTitle *string
	TeacherName *string
	SubmittedAt *time.Time
}

// Repository is the persistence contract for teaching data; the service
// depends on this interface, tests supply a fake.
//
// Reads are deliberately NOT teacher-filtered (only center-filtered): the
// class resolution through classes.Get is the authorization gate (it hides
// other teachers' classes from non-owners), and teaching rows keep their
// creation-time teacher_id, so a teacher-filter here would orphan a class's
// curriculum and plans the moment the class changed hands.
type Repository interface {
	// GetCurriculum returns the class's curriculum row, or nil (not an
	// error) when the class has none — the service maps that to the empty
	// default the UI expects.
	GetCurriculum(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*Curriculum, error)
	// UpsertCurriculum whole-replaces the class's lessons and progress
	// pointer, keeping the row id and creation-time teacher anchor stable
	// across edits.
	UpsertCurriculum(ctx context.Context, cur *Curriculum) error
	// ListPlans returns every saved plan of a class, ordered by lesson
	// index. A class with no plans returns an empty slice.
	ListPlans(ctx context.Context, sc authctx.Scope, classID uuid.UUID) ([]Plan, error)
	// GetPlan returns one plan by (class, lesson index), or nil when never
	// saved — the state machine reads that as StatusNone.
	GetPlan(ctx context.Context, sc authctx.Scope, classID uuid.UUID, lessonIndex int) (*Plan, error)
	// CreatePlan inserts a first save. A concurrent duplicate save loses to
	// uq_lesson_plans_class_lesson and surfaces as an error.
	CreatePlan(ctx context.Context, plan *Plan) error
	// UpdatePlan writes the full row back. The service always mutates a row
	// it just loaded, so a whole-row update is the simplest correct write —
	// it can clear nullable columns (redo_note, owner_comment) that a
	// partial struct update would silently skip.
	UpdatePlan(ctx context.Context, plan *Plan) error
	// ReviewQueue lists the center's pending plans with their display
	// context, oldest submission first — served by idx_lesson_plans_pending.
	ReviewQueue(ctx context.Context, sc authctx.Scope) ([]QueueRow, error)
	// TeacherNames resolves display names for submitter ids. No center
	// filter: teachers are global identities and the ids come from rows the
	// caller already resolved through their own scope.
	TeacherNames(ctx context.Context, teacherIDs []uuid.UUID) (map[uuid.UUID]string, error)
	// ListNotesForClassMonth returns every session note of the class whose
	// session falls in [from, to) — one join, no per-session queries.
	ListNotesForClassMonth(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]SessionNote, error)
	// ListMarksForClassMonth is the marks counterpart of
	// ListNotesForClassMonth.
	ListMarksForClassMonth(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]SessionMark, error)
	// UpsertNote whole-replaces the session's note body (1:1 by session_id).
	UpsertNote(ctx context.Context, note *SessionNote) error
	// DeleteNote removes the session's note; deleting a non-existent note is
	// a no-op (the empty-body write is idempotent).
	DeleteNote(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) error
	// ListMarksBySession returns the session's current mark rows — the base
	// the batch write merges into.
	ListMarksBySession(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]SessionMark, error)
	// UpsertMarks batch-writes mark rows in one statement, keyed on
	// uq_session_marks_session_student.
	UpsertMarks(ctx context.Context, marks []SessionMark) error
	// DeleteMarks removes the given students' rows for the session — the
	// both-fields-NULL outcome of the merge.
	DeleteMarks(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, studentIDs []uuid.UUID) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) GetCurriculum(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*Curriculum, error) {
	var cur Curriculum
	err := database.FromContext(ctx, r.db).
		Where("center_id = ? AND class_id = ?", sc.CenterID, classID).
		Take(&cur).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cur, nil
}

func (r *gormRepository) UpsertCurriculum(ctx context.Context, cur *Curriculum) error {
	// updated_at gets a fresh now() rather than the excluded value so the
	// row reflects the edit time; id and the teacher/center anchors survive
	// every edit.
	return database.FromContext(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "class_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"lessons":       gorm.Expr("excluded.lessons"),
				"current_index": gorm.Expr("excluded.current_index"),
				"updated_at":    gorm.Expr("now()"),
			}),
		}).
		Create(cur).Error
}

func (r *gormRepository) ListPlans(ctx context.Context, sc authctx.Scope, classID uuid.UUID) ([]Plan, error) {
	var plans []Plan
	err := database.FromContext(ctx, r.db).
		Where("center_id = ? AND class_id = ?", sc.CenterID, classID).
		Order("lesson_index").
		Find(&plans).Error
	return plans, err
}

func (r *gormRepository) GetPlan(ctx context.Context, sc authctx.Scope, classID uuid.UUID, lessonIndex int) (*Plan, error) {
	var plan Plan
	err := database.FromContext(ctx, r.db).
		Where("center_id = ? AND class_id = ? AND lesson_index = ?", sc.CenterID, classID, lessonIndex).
		Take(&plan).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func (r *gormRepository) CreatePlan(ctx context.Context, plan *Plan) error {
	return database.FromContext(ctx, r.db).Create(plan).Error
}

func (r *gormRepository) UpdatePlan(ctx context.Context, plan *Plan) error {
	return database.FromContext(ctx, r.db).Save(plan).Error
}

func (r *gormRepository) ReviewQueue(ctx context.Context, sc authctx.Scope) ([]QueueRow, error) {
	var rows []QueueRow
	// lessons -> lesson_index extracts the plan's lesson title from the
	// curriculum's JSONB array; both that join and the teachers join are
	// LEFT so a shrunken/missing curriculum or nameless submitter yields
	// NULL instead of dropping the queue row.
	err := database.FromContext(ctx, r.db).
		Table("lesson_plans").
		Select(`lesson_plans.id AS plan_id,
			lesson_plans.class_id,
			classes.name AS class_name,
			lesson_plans.lesson_index,
			class_curricula.lessons ->> lesson_plans.lesson_index AS lesson_title,
			teachers.full_name AS teacher_name,
			lesson_plans.submitted_at`).
		Joins("JOIN classes ON classes.id = lesson_plans.class_id AND classes.center_id = lesson_plans.center_id").
		Joins("LEFT JOIN class_curricula ON class_curricula.class_id = lesson_plans.class_id").
		Joins("LEFT JOIN teachers ON teachers.id = lesson_plans.submitted_by").
		Where("lesson_plans.center_id = ? AND lesson_plans.status = ?", sc.CenterID, StatusPending).
		Order("lesson_plans.submitted_at").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) ListNotesForClassMonth(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]SessionNote, error) {
	var notes []SessionNote
	err := database.FromContext(ctx, r.db).
		Joins("JOIN class_sessions ON class_sessions.id = session_notes.session_id AND class_sessions.center_id = session_notes.center_id").
		Where("session_notes.center_id = ? AND class_sessions.class_id = ? AND class_sessions.session_date >= ? AND class_sessions.session_date < ?",
			sc.CenterID, classID, from, to).
		Find(&notes).Error
	return notes, err
}

func (r *gormRepository) ListMarksForClassMonth(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]SessionMark, error) {
	var marks []SessionMark
	err := database.FromContext(ctx, r.db).
		Joins("JOIN class_sessions ON class_sessions.id = session_marks.session_id AND class_sessions.center_id = session_marks.center_id").
		Where("session_marks.center_id = ? AND class_sessions.class_id = ? AND class_sessions.session_date >= ? AND class_sessions.session_date < ?",
			sc.CenterID, classID, from, to).
		Find(&marks).Error
	return marks, err
}

func (r *gormRepository) UpsertNote(ctx context.Context, note *SessionNote) error {
	return database.FromContext(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "session_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"body":       gorm.Expr("excluded.body"),
				"updated_at": gorm.Expr("now()"),
			}),
		}).
		Create(note).Error
}

func (r *gormRepository) DeleteNote(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) error {
	return database.FromContext(ctx, r.db).
		Where("center_id = ? AND session_id = ?", sc.CenterID, sessionID).
		Delete(&SessionNote{}).Error
}

func (r *gormRepository) ListMarksBySession(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]SessionMark, error) {
	var marks []SessionMark
	err := database.FromContext(ctx, r.db).
		Where("center_id = ? AND session_id = ?", sc.CenterID, sessionID).
		Find(&marks).Error
	return marks, err
}

func (r *gormRepository) UpsertMarks(ctx context.Context, marks []SessionMark) error {
	if len(marks) == 0 {
		return nil
	}
	return database.FromContext(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "session_id"}, {Name: "student_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"score":         gorm.Expr("excluded.score"),
				"personal_note": gorm.Expr("excluded.personal_note"),
				"updated_at":    gorm.Expr("now()"),
			}),
		}).
		Create(marks).Error
}

func (r *gormRepository) DeleteMarks(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, studentIDs []uuid.UUID) error {
	if len(studentIDs) == 0 {
		return nil
	}
	return database.FromContext(ctx, r.db).
		Where("center_id = ? AND session_id = ? AND student_id IN ?", sc.CenterID, sessionID, studentIDs).
		Delete(&SessionMark{}).Error
}

func (r *gormRepository) TeacherNames(ctx context.Context, teacherIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string, len(teacherIDs))
	if len(teacherIDs) == 0 {
		return out, nil
	}
	type row struct {
		ID       uuid.UUID
		FullName string
	}
	var rows []row
	err := database.FromContext(ctx, r.db).
		Table("teachers").
		Select("id, full_name").
		Where("id IN ?", teacherIDs).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, rr := range rows {
		out[rr.ID] = rr.FullName
	}
	return out, nil
}
