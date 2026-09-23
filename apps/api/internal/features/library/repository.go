package library

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/likeq"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows ListTemplates. Q matches the code or name.
type ListFilter struct {
	Q string
}

// ItemUsage counts the lesson links of a material or exercise held by
// live templates, split by whether the version is still a draft (the
// author can detach) or released (published or archived, final).
type ItemUsage struct {
	Draft    int64
	Released int64
}

// Repository is the persistence contract for the library. Every method is
// bound to the caller's center: a template, version or lesson of another
// center reads as ErrNotFound. The library is center-wide by design, so no
// method narrows further by teacher.
type Repository interface {
	// CreateTemplate inserts a template; gorm.ErrDuplicatedKey when a live
	// template of the center already uses the code.
	CreateTemplate(ctx context.Context, sc authctx.Scope, t *Template) error
	// GetTemplate loads one live template with its version summary.
	GetTemplate(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*TemplateRow, error)
	// ListTemplates pages the center's live templates.
	ListTemplates(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]TemplateRow, int64, error)
	// UpdateTemplate replaces the template's own fields;
	// gorm.ErrDuplicatedKey on a code clash, ErrNotFound when missing.
	UpdateTemplate(ctx context.Context, sc authctx.Scope, t *Template) error
	// SoftDeleteTemplate stamps deleted_at; ErrNotFound when missing.
	SoftDeleteTemplate(ctx context.Context, sc authctx.Scope, id uuid.UUID) error

	// CreateVersion inserts a version; gorm.ErrDuplicatedKey when the
	// template already has a draft (uq_program_template_versions_draft).
	CreateVersion(ctx context.Context, sc authctx.Scope, v *Version) error
	// GetVersion loads one version of the center with its lesson count.
	GetVersion(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*VersionRow, error)
	// LockVersion loads the version FOR UPDATE inside the caller's
	// transaction so writes to it and its lessons serialise with each
	// other and with a concurrent publish. ErrNotFound when the version is
	// outside the center or its template was deleted: writes to a deleted
	// template's versions are refused even though reads still work.
	LockVersion(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Version, error)
	// ListVersions returns the template's versions, newest first.
	ListVersions(ctx context.Context, sc authctx.Scope, templateID uuid.UUID) ([]VersionRow, error)
	// LatestReleasedVersion returns the highest non-draft version of the
	// template (published or archived), or ErrNotFound when every version
	// so far is a draft.
	LatestReleasedVersion(ctx context.Context, sc authctx.Scope, templateID uuid.UUID) (*Version, error)
	// NextVersionNo returns max(version_no)+1 for the template.
	NextVersionNo(ctx context.Context, sc authctx.Scope, templateID uuid.UUID) (int, error)
	// SetVersionStatus moves the version from status `from` to `to`,
	// stamping published_at when given; ErrNotFound when no row of the
	// center is in `from`.
	SetVersionStatus(ctx context.Context, sc authctx.Scope, id uuid.UUID, from, to string, publishedAt *time.Time) error

	// ListLessons returns the version's lessons by position.
	ListLessons(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) ([]Lesson, error)
	// GetLesson loads one lesson of the center.
	GetLesson(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Lesson, error)
	// NextPosition returns max(position)+1 for the version. Call it with
	// the version locked: two unlocked callers would compute the same slot.
	NextPosition(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (int, error)
	// CreateLessons inserts the rows in one statement.
	CreateLessons(ctx context.Context, sc authctx.Scope, rows []*Lesson) error
	// UpdateLesson replaces the lesson's content; ErrNotFound when missing.
	UpdateLesson(ctx context.Context, sc authctx.Scope, l *Lesson) error
	// DeleteLesson removes one lesson; ErrNotFound when missing.
	DeleteLesson(ctx context.Context, sc authctx.Scope, id uuid.UUID) error
	// SetPositions renumbers the version's lessons 1..n in the given order.
	// It must run inside a transaction: the unique on (version_id, position)
	// is deferred, so intermediate duplicates are fine until commit.
	SetPositions(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, ids []uuid.UUID) error

	// CreateMaterial inserts a material of the center.
	CreateMaterial(ctx context.Context, sc authctx.Scope, m *Material) error
	// GetMaterial loads one live material; ErrNotFound when missing.
	GetMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Material, error)
	// ListMaterials pages the center's live materials; Q matches the title.
	ListMaterials(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]Material, int64, error)
	// FindMaterials returns the live materials of the center among ids, in
	// no particular order, and holds each row shared until the transaction
	// ends so a concurrent delete waits. Fewer rows than ids means some
	// are unknown.
	FindMaterials(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) ([]Material, error)
	// LockMaterial loads one live material and holds its row for update
	// until the transaction ends; ErrNotFound when missing.
	LockMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Material, error)
	// UpdateMaterial replaces the material's fields; ErrNotFound when missing.
	UpdateMaterial(ctx context.Context, sc authctx.Scope, m *Material) error
	// SoftDeleteMaterial stamps deleted_at; ErrNotFound when missing.
	SoftDeleteMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID) error
	// MaterialUsage counts the lesson links of live templates that still
	// point at the material.
	MaterialUsage(ctx context.Context, sc authctx.Scope, id uuid.UUID) (ItemUsage, error)

	// CreateExercise inserts an exercise of the center.
	CreateExercise(ctx context.Context, sc authctx.Scope, e *Exercise) error
	// GetExercise loads one live exercise; ErrNotFound when missing.
	GetExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Exercise, error)
	// ListExercises pages the center's live exercises; Q matches the title.
	ListExercises(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]Exercise, int64, error)
	// FindExercises returns the live exercises of the center among ids,
	// held shared like FindMaterials.
	FindExercises(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) ([]Exercise, error)
	// LockExercise loads one live exercise and holds its row for update.
	LockExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Exercise, error)
	// UpdateExercise replaces the exercise's fields; ErrNotFound when missing.
	UpdateExercise(ctx context.Context, sc authctx.Scope, e *Exercise) error
	// SoftDeleteExercise stamps deleted_at; ErrNotFound when missing.
	SoftDeleteExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID) error
	// ExerciseUsage counts the lesson links of live templates that still
	// point at the exercise.
	ExerciseUsage(ctx context.Context, sc authctx.Scope, id uuid.UUID) (ItemUsage, error)

	// ListLessonMaterials returns the materials attached to the given
	// lessons, ordered by lesson then position.
	ListLessonMaterials(ctx context.Context, sc authctx.Scope, lessonIDs []uuid.UUID) ([]LessonMaterialRow, error)
	// ReplaceLessonMaterials swaps the lesson's whole material list for
	// rows (positions assigned 1..n in order). Run it with the lesson's
	// version locked.
	ReplaceLessonMaterials(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, rows []LessonMaterial) error
	// CreateLessonMaterials inserts links as given (lesson and position
	// set by the caller) in one statement, for copying a version.
	CreateLessonMaterials(ctx context.Context, sc authctx.Scope, rows []LessonMaterial) error
	// ListLessonExercises returns the exercises attached to the given
	// lessons, ordered by lesson then position.
	ListLessonExercises(ctx context.Context, sc authctx.Scope, lessonIDs []uuid.UUID) ([]LessonExerciseRow, error)
	// ReplaceLessonExercises swaps the lesson's whole exercise list.
	ReplaceLessonExercises(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, rows []LessonExercise) error
	// CreateLessonExercises inserts links as given in one statement.
	CreateLessonExercises(ctx context.Context, sc authctx.Scope, rows []LessonExercise) error

	// ListLogFields returns the version's log fields by position.
	ListLogFields(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) ([]LogField, error)
	// ReplaceLogFields swaps the version's whole log-field list for rows
	// (positions assigned 1..n in order). Run it with the version locked
	// and inside a transaction: the unique on (version_id, position) is
	// deferred.
	ReplaceLogFields(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, rows []*LogField) error
	// SetScoreSet replaces the version's score set; ErrNotFound when the
	// version is outside the center.
	SetScoreSet(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, set ScoreSet) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

const templateSummarySelect = `program_templates.*,
	(SELECT max(v.version_no) FROM program_template_versions v
	  WHERE v.template_id = program_templates.id AND v.status = 'published') AS published_version_no,
	(SELECT v.version_no FROM program_template_versions v
	  WHERE v.template_id = program_templates.id AND v.status = 'draft') AS draft_version_no,
	(SELECT count(*) FROM program_template_versions v
	  WHERE v.template_id = program_templates.id) AS version_count`

// liveTemplates scopes a query to the caller's center's non-deleted templates.
func (r *gormRepository) liveTemplates(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&Template{}).
		Where("program_templates.center_id = ? AND program_templates.deleted_at IS NULL", sc.CenterID)
}

func (r *gormRepository) CreateTemplate(ctx context.Context, sc authctx.Scope, t *Template) error {
	if t.ID == uuid.Nil {
		t.ID = id.New()
	}
	t.CenterID = sc.CenterID
	return database.FromContext(ctx, r.db).Create(t).Error
}

func (r *gormRepository) GetTemplate(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*TemplateRow, error) {
	var row TemplateRow
	err := r.liveTemplates(ctx, sc).
		Select(templateSummarySelect).
		Where("program_templates.id = ?", id).
		Take(&row).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &row, nil
}

func (r *gormRepository) ListTemplates(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]TemplateRow, int64, error) {
	q := r.liveTemplates(ctx, sc)
	if f.Q != "" {
		needle := likeq.Contains(f.Q)
		q = q.Where(`(program_templates.name ILIKE ? ESCAPE '\' OR program_templates.code ILIKE ? ESCAPE '\')`, needle, needle)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []TemplateRow
	// The id tie-breaker keeps pages stable when several templates share
	// the sort value (same name, same creation second).
	err := q.Select(templateSummarySelect).Scopes(p.Scope).Order("program_templates.id ASC").Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) UpdateTemplate(ctx context.Context, sc authctx.Scope, t *Template) error {
	res := r.liveTemplates(ctx, sc).
		Where("program_templates.id = ?", t.ID).
		Updates(map[string]any{
			"code": t.Code, "name": t.Name, "subject": t.Subject, "level": t.Level,
			"description": t.Description, "updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SoftDeleteTemplate(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.liveTemplates(ctx, sc).
		Where("program_templates.id = ?", id).
		Updates(map[string]any{"deleted_at": gorm.Expr("now()"), "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

const versionSelect = `program_template_versions.*,
	(SELECT count(*) FROM template_lessons l
	  WHERE l.version_id = program_template_versions.id) AS lesson_count`

func (r *gormRepository) versions(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&Version{}).
		Where("program_template_versions.center_id = ?", sc.CenterID)
}

func (r *gormRepository) CreateVersion(ctx context.Context, sc authctx.Scope, v *Version) error {
	if v.ID == uuid.Nil {
		v.ID = id.New()
	}
	v.CenterID = sc.CenterID
	return database.FromContext(ctx, r.db).Create(v).Error
}

func (r *gormRepository) GetVersion(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*VersionRow, error) {
	var row VersionRow
	err := r.versions(ctx, sc).
		Select(versionSelect).
		Where("program_template_versions.id = ?", id).
		Take(&row).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &row, nil
}

func (r *gormRepository) LockVersion(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Version, error) {
	var v Version
	// FOR UPDATE OF the version table only: the joined template row is
	// read, not locked, so template edits do not queue behind lesson writes.
	err := r.versions(ctx, sc).
		Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "program_template_versions"}}).
		Joins("JOIN program_templates t ON t.id = program_template_versions.template_id AND t.deleted_at IS NULL").
		Where("program_template_versions.id = ?", id).
		Take(&v).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &v, nil
}

func (r *gormRepository) ListVersions(ctx context.Context, sc authctx.Scope, templateID uuid.UUID) ([]VersionRow, error) {
	var rows []VersionRow
	err := r.versions(ctx, sc).
		Select(versionSelect).
		Where("program_template_versions.template_id = ?", templateID).
		Order("program_template_versions.version_no DESC").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) LatestReleasedVersion(ctx context.Context, sc authctx.Scope, templateID uuid.UUID) (*Version, error) {
	var v Version
	err := r.versions(ctx, sc).
		Where("template_id = ? AND status <> ?", templateID, StatusDraft).
		Order("version_no DESC").
		Take(&v).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &v, nil
}

func (r *gormRepository) NextVersionNo(ctx context.Context, sc authctx.Scope, templateID uuid.UUID) (int, error) {
	var next int
	err := r.versions(ctx, sc).
		Where("template_id = ?", templateID).
		Select("COALESCE(max(version_no), 0) + 1").
		Scan(&next).Error
	return next, err
}

func (r *gormRepository) SetVersionStatus(ctx context.Context, sc authctx.Scope, id uuid.UUID, from, to string, publishedAt *time.Time) error {
	fields := map[string]any{"status": to, "updated_at": gorm.Expr("now()")}
	if publishedAt != nil {
		fields["published_at"] = *publishedAt
	}
	res := r.versions(ctx, sc).
		Where("id = ? AND status = ?", id, from).
		Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) lessons(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&Lesson{}).
		Where("template_lessons.center_id = ?", sc.CenterID)
}

func (r *gormRepository) ListLessons(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) ([]Lesson, error) {
	var rows []Lesson
	err := r.lessons(ctx, sc).
		Where("version_id = ?", versionID).
		Order("position ASC").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) GetLesson(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Lesson, error) {
	var l Lesson
	err := r.lessons(ctx, sc).Where("id = ?", id).Take(&l).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &l, nil
}

func (r *gormRepository) NextPosition(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (int, error) {
	var next int
	err := r.lessons(ctx, sc).
		Where("version_id = ?", versionID).
		Select("COALESCE(max(position), 0) + 1").
		Scan(&next).Error
	return next, err
}

func (r *gormRepository) CreateLessons(ctx context.Context, sc authctx.Scope, rows []*Lesson) error {
	if len(rows) == 0 {
		return nil
	}
	for _, l := range rows {
		if l.ID == uuid.Nil {
			l.ID = id.New()
		}
		l.CenterID = sc.CenterID
	}
	return database.FromContext(ctx, r.db).Create(rows).Error
}

func (r *gormRepository) UpdateLesson(ctx context.Context, sc authctx.Scope, l *Lesson) error {
	res := r.lessons(ctx, sc).
		Where("id = ?", l.ID).
		Updates(map[string]any{
			"title": l.Title, "objectives": l.Objectives, "duration_min": l.DurationMin,
			"homework_note": l.HomeworkNote, "updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) DeleteLesson(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.lessons(ctx, sc).Where("id = ?", id).Delete(&Lesson{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SetPositions(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, ids []uuid.UUID) error {
	db := database.FromContext(ctx, r.db)
	for i, id := range ids {
		res := db.Model(&Lesson{}).
			Where("center_id = ? AND version_id = ? AND id = ?", sc.CenterID, versionID, id).
			Updates(map[string]any{"position": i + 1, "updated_at": gorm.Expr("now()")})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
	}
	return nil
}

// materials scopes a query to the caller's center's live materials.
func (r *gormRepository) materials(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&Material{}).
		Where("library_materials.center_id = ? AND library_materials.deleted_at IS NULL", sc.CenterID)
}

func (r *gormRepository) CreateMaterial(ctx context.Context, sc authctx.Scope, m *Material) error {
	if m.ID == uuid.Nil {
		m.ID = id.New()
	}
	m.CenterID = sc.CenterID
	return database.FromContext(ctx, r.db).Create(m).Error
}

func (r *gormRepository) GetMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Material, error) {
	var m Material
	if err := r.materials(ctx, sc).Where("library_materials.id = ?", id).Take(&m).Error; err != nil {
		return nil, notFoundOr(err)
	}
	return &m, nil
}

func (r *gormRepository) ListMaterials(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]Material, int64, error) {
	q := r.materials(ctx, sc)
	if f.Q != "" {
		q = q.Where(`library_materials.title ILIKE ? ESCAPE '\'`, likeq.Contains(f.Q))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Material
	err := q.Scopes(p.Scope).Order("library_materials.id ASC").Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) FindMaterials(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) ([]Material, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []Material
	// FOR SHARE: a delete of one of these rows (FOR UPDATE) queues behind
	// this transaction and then sees the link it writes.
	err := r.materials(ctx, sc).
		Clauses(clause.Locking{Strength: "SHARE"}).
		Where("library_materials.id IN ?", ids).
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) LockMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Material, error) {
	var m Material
	err := r.materials(ctx, sc).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("library_materials.id = ?", id).
		Take(&m).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &m, nil
}

func (r *gormRepository) UpdateMaterial(ctx context.Context, sc authctx.Scope, m *Material) error {
	res := r.materials(ctx, sc).
		Where("library_materials.id = ?", m.ID).
		Updates(map[string]any{
			"title": m.Title, "kind": m.Kind, "url": m.URL, "description": m.Description,
			"tags": m.Tags, "updated_at": gorm.Expr("now()"),
		})
	return affected(res)
}

func (r *gormRepository) SoftDeleteMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.materials(ctx, sc).
		Where("library_materials.id = ?", id).
		Updates(map[string]any{"deleted_at": gorm.Expr("now()"), "updated_at": gorm.Expr("now()")})
	return affected(res)
}

func (r *gormRepository) MaterialUsage(ctx context.Context, sc authctx.Scope, id uuid.UUID) (ItemUsage, error) {
	var u ItemUsage
	// Links held by a soft-deleted template are unreachable, so they do
	// not count; the template's rows are kept only for the soft delete.
	err := r.lessonMaterials(ctx, sc).
		Select("count(*) FILTER (WHERE v.status = ?) AS draft, count(*) FILTER (WHERE v.status <> ?) AS released", StatusDraft, StatusDraft).
		Joins("JOIN template_lessons l ON l.id = template_lesson_materials.lesson_id").
		Joins("JOIN program_template_versions v ON v.id = l.version_id").
		Joins("JOIN program_templates t ON t.id = v.template_id AND t.deleted_at IS NULL").
		Where("template_lesson_materials.material_id = ?", id).
		Scan(&u).Error
	return u, err
}

// exercises scopes a query to the caller's center's live exercises.
func (r *gormRepository) exercises(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&Exercise{}).
		Where("library_exercises.center_id = ? AND library_exercises.deleted_at IS NULL", sc.CenterID)
}

func (r *gormRepository) CreateExercise(ctx context.Context, sc authctx.Scope, e *Exercise) error {
	if e.ID == uuid.Nil {
		e.ID = id.New()
	}
	e.CenterID = sc.CenterID
	return database.FromContext(ctx, r.db).Create(e).Error
}

func (r *gormRepository) GetExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Exercise, error) {
	var e Exercise
	if err := r.exercises(ctx, sc).Where("library_exercises.id = ?", id).Take(&e).Error; err != nil {
		return nil, notFoundOr(err)
	}
	return &e, nil
}

func (r *gormRepository) ListExercises(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]Exercise, int64, error) {
	q := r.exercises(ctx, sc)
	if f.Q != "" {
		q = q.Where(`library_exercises.title ILIKE ? ESCAPE '\'`, likeq.Contains(f.Q))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Exercise
	err := q.Scopes(p.Scope).Order("library_exercises.id ASC").Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) FindExercises(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) ([]Exercise, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []Exercise
	err := r.exercises(ctx, sc).
		Clauses(clause.Locking{Strength: "SHARE"}).
		Where("library_exercises.id IN ?", ids).
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) LockExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Exercise, error) {
	var e Exercise
	err := r.exercises(ctx, sc).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("library_exercises.id = ?", id).
		Take(&e).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &e, nil
}

func (r *gormRepository) UpdateExercise(ctx context.Context, sc authctx.Scope, e *Exercise) error {
	res := r.exercises(ctx, sc).
		Where("library_exercises.id = ?", e.ID).
		Updates(map[string]any{
			"title": e.Title, "description": e.Description, "difficulty": e.Difficulty,
			"tags": e.Tags, "updated_at": gorm.Expr("now()"),
		})
	return affected(res)
}

func (r *gormRepository) SoftDeleteExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.exercises(ctx, sc).
		Where("library_exercises.id = ?", id).
		Updates(map[string]any{"deleted_at": gorm.Expr("now()"), "updated_at": gorm.Expr("now()")})
	return affected(res)
}

func (r *gormRepository) ExerciseUsage(ctx context.Context, sc authctx.Scope, id uuid.UUID) (ItemUsage, error) {
	var u ItemUsage
	err := r.lessonExercises(ctx, sc).
		Select("count(*) FILTER (WHERE v.status = ?) AS draft, count(*) FILTER (WHERE v.status <> ?) AS released", StatusDraft, StatusDraft).
		Joins("JOIN template_lessons l ON l.id = template_lesson_exercises.lesson_id").
		Joins("JOIN program_template_versions v ON v.id = l.version_id").
		Joins("JOIN program_templates t ON t.id = v.template_id AND t.deleted_at IS NULL").
		Where("template_lesson_exercises.exercise_id = ?", id).
		Scan(&u).Error
	return u, err
}

// lessonMaterials scopes a query to the center's lesson↔material links.
func (r *gormRepository) lessonMaterials(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&LessonMaterial{}).
		Where("template_lesson_materials.center_id = ?", sc.CenterID)
}

func (r *gormRepository) ListLessonMaterials(ctx context.Context, sc authctx.Scope, lessonIDs []uuid.UUID) ([]LessonMaterialRow, error) {
	if len(lessonIDs) == 0 {
		return nil, nil
	}
	var rows []LessonMaterialRow
	// The link is what says the material belongs here; the material row is
	// read as-is because a material linked from a live template can never
	// be deleted (only a deleted template may keep links to deleted rows).
	err := r.lessonMaterials(ctx, sc).
		Select("m.*, template_lesson_materials.lesson_id, template_lesson_materials.shared_with_students, template_lesson_materials.position").
		Joins("JOIN library_materials m ON m.id = template_lesson_materials.material_id").
		Where("template_lesson_materials.lesson_id IN ?", lessonIDs).
		Order("template_lesson_materials.lesson_id, template_lesson_materials.position").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) ReplaceLessonMaterials(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, rows []LessonMaterial) error {
	if err := r.lessonMaterials(ctx, sc).Where("lesson_id = ?", lessonID).Delete(&LessonMaterial{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].LessonID, rows[i].CenterID, rows[i].Position = lessonID, sc.CenterID, i+1
	}
	return database.FromContext(ctx, r.db).Create(&rows).Error
}

func (r *gormRepository) CreateLessonMaterials(ctx context.Context, sc authctx.Scope, rows []LessonMaterial) error {
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].CenterID = sc.CenterID
	}
	return database.FromContext(ctx, r.db).Create(&rows).Error
}

// lessonExercises scopes a query to the center's lesson↔exercise links.
func (r *gormRepository) lessonExercises(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&LessonExercise{}).
		Where("template_lesson_exercises.center_id = ?", sc.CenterID)
}

func (r *gormRepository) ListLessonExercises(ctx context.Context, sc authctx.Scope, lessonIDs []uuid.UUID) ([]LessonExerciseRow, error) {
	if len(lessonIDs) == 0 {
		return nil, nil
	}
	var rows []LessonExerciseRow
	err := r.lessonExercises(ctx, sc).
		Select("e.*, template_lesson_exercises.lesson_id, template_lesson_exercises.position").
		Joins("JOIN library_exercises e ON e.id = template_lesson_exercises.exercise_id").
		Where("template_lesson_exercises.lesson_id IN ?", lessonIDs).
		Order("template_lesson_exercises.lesson_id, template_lesson_exercises.position").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) ReplaceLessonExercises(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, rows []LessonExercise) error {
	if err := r.lessonExercises(ctx, sc).Where("lesson_id = ?", lessonID).Delete(&LessonExercise{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].LessonID, rows[i].CenterID, rows[i].Position = lessonID, sc.CenterID, i+1
	}
	return database.FromContext(ctx, r.db).Create(&rows).Error
}

func (r *gormRepository) CreateLessonExercises(ctx context.Context, sc authctx.Scope, rows []LessonExercise) error {
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].CenterID = sc.CenterID
	}
	return database.FromContext(ctx, r.db).Create(&rows).Error
}

// logFields scopes a query to the center's template log fields.
func (r *gormRepository) logFields(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&LogField{}).
		Where("template_log_fields.center_id = ?", sc.CenterID)
}

func (r *gormRepository) ListLogFields(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) ([]LogField, error) {
	var rows []LogField
	err := r.logFields(ctx, sc).
		Where("version_id = ?", versionID).
		Order("position ASC").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) ReplaceLogFields(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, rows []*LogField) error {
	if err := r.logFields(ctx, sc).Where("version_id = ?", versionID).Delete(&LogField{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	for i, f := range rows {
		if f.ID == uuid.Nil {
			f.ID = id.New()
		}
		f.VersionID, f.CenterID, f.Position = versionID, sc.CenterID, i+1
	}
	return database.FromContext(ctx, r.db).Create(rows).Error
}

func (r *gormRepository) SetScoreSet(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, set ScoreSet) error {
	res := r.versions(ctx, sc).
		Where("id = ?", versionID).
		Updates(map[string]any{"score_set": set, "updated_at": gorm.Expr("now()")})
	return affected(res)
}

// affected turns a zero-row update into ErrNotFound.
func affected(res *gorm.DB) error {
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// notFoundOr maps GORM's not-found sentinel onto the package's own.
func notFoundOr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
