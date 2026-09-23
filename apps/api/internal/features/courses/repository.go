package courses

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/library"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/likeq"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows List. Status keeps one status; Q matches the code or
// name.
type ListFilter struct {
	Status string
	Q      string
}

// Repository is the persistence contract for the catalog. Every method is
// bound to the caller's center: a course of another center reads as
// ErrNotFound. The catalog is center-wide by design, so no method narrows
// further by teacher.
type Repository interface {
	// Create inserts a course; gorm.ErrDuplicatedKey when a live course of
	// the center already uses the code.
	Create(ctx context.Context, sc authctx.Scope, c *Course) error
	// Get loads one live course with its derived counters.
	Get(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*CourseRow, error)
	// Lock loads one live course FOR UPDATE inside the ambient transaction,
	// so a class attaching to it (FOR SHARE) and a delete or pack rewrite
	// serialise on the row; ErrNotFound when missing.
	Lock(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Course, error)
	// List pages the center's live courses.
	List(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]CourseRow, int64, error)
	// Update replaces the course's own fields; gorm.ErrDuplicatedKey on a
	// code clash, ErrNotFound when missing.
	Update(ctx context.Context, sc authctx.Scope, c *Course) error
	// SetStatus moves a live course to status; ErrNotFound when missing.
	SetStatus(ctx context.Context, sc authctx.Scope, id uuid.UUID, status string) error
	// SoftDelete stamps deleted_at; ErrNotFound when missing.
	SoftDelete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error
	// LiveClassCount counts the non-deleted classes attached to the course.
	LiveClassCount(ctx context.Context, sc authctx.Scope, id uuid.UUID) (int64, error)
	// PathCount counts the stages of live learning paths that recommend
	// the course.
	PathCount(ctx context.Context, sc authctx.Scope, id uuid.UUID) (int64, error)
	// ListPaths returns the stages of live learning paths that recommend
	// the course, ordered by path name then stage position.
	ListPaths(ctx context.Context, sc authctx.Scope, id uuid.UUID) ([]CoursePath, error)
	// FindTemplateVersion loads one program template version of the center
	// (any status) with its template's code and name; ErrNotFound when the
	// version is outside the center or its template was deleted.
	FindTemplateVersion(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (*TemplateVersionRef, error)
	// ListPacks returns the tuition packs of the given courses ordered by
	// course then position.
	ListPacks(ctx context.Context, sc authctx.Scope, courseIDs []uuid.UUID) ([]TuitionPack, error)
	// ReplacePacks swaps the course's whole pack list for rows (positions
	// assigned 1..n in order). Run it inside a transaction: the unique on
	// (course_id, position) is deferred.
	ReplacePacks(ctx context.Context, sc authctx.Scope, courseID uuid.UUID, rows []*TuitionPack) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// live scopes a query to the caller's center's non-deleted courses.
func (r *gormRepository) live(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&Course{}).
		Where("courses.center_id = ? AND courses.deleted_at IS NULL", sc.CenterID)
}

// summary is the select list of a CourseRow: the class counters share the
// phase rules of the class list through classes.PhasePredicate so the two
// cannot drift, and the default template is joined for display.
func summary(q *gorm.DB) *gorm.DB {
	today := classes.Today()
	running, runningArgs, _ := classes.PhasePredicate(classes.PhaseRunning, today)
	upcoming, upcomingArgs, _ := classes.PhasePredicate(classes.PhaseUpcoming, today)
	const classesOf = "SELECT count(*) FROM classes WHERE classes.course_id = courses.id AND classes.deleted_at IS NULL AND "
	args := append(append([]any{}, runningArgs...), upcomingArgs...)
	return q.
		Select(`courses.*,
			(`+classesOf+running+`) AS classes_running,
			(`+classesOf+upcoming+`) AS classes_upcoming,
			t.id AS template_id, v.version_no AS template_version_no, v.status AS template_version_status,
			t.code AS template_code, t.name AS template_name`, args...).
		Joins("LEFT JOIN program_template_versions v ON v.id = courses.default_template_version_id").
		// A deleted template drops out of the embed (the stored id stays on
		// the row); the client then offers to pick another one.
		Joins("LEFT JOIN program_templates t ON t.id = v.template_id AND t.deleted_at IS NULL")
}

func (r *gormRepository) Create(ctx context.Context, sc authctx.Scope, c *Course) error {
	if c.ID == uuid.Nil {
		c.ID = id.New()
	}
	c.CenterID = sc.CenterID
	return database.FromContext(ctx, r.db).Create(c).Error
}

func (r *gormRepository) Get(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*CourseRow, error) {
	var row CourseRow
	err := summary(r.live(ctx, sc)).Where("courses.id = ?", id).Take(&row).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &row, nil
}

func (r *gormRepository) Lock(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Course, error) {
	var c Course
	err := r.live(ctx, sc).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("courses.id = ?", id).
		Take(&c).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &c, nil
}

func (r *gormRepository) List(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]CourseRow, int64, error) {
	q := r.live(ctx, sc)
	if f.Status != "" {
		q = q.Where("courses.status = ?", f.Status)
	}
	if f.Q != "" {
		needle := likeq.Contains(f.Q)
		q = q.Where(`(courses.name ILIKE ? ESCAPE '\' OR courses.code ILIKE ? ESCAPE '\')`, needle, needle)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []CourseRow
	// The id tie-breaker keeps pages stable when several courses share
	// the sort value.
	err := summary(q).Scopes(p.Scope).Order("courses.id ASC").Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) Update(ctx context.Context, sc authctx.Scope, c *Course) error {
	res := r.live(ctx, sc).
		Where("courses.id = ?", c.ID).
		Updates(map[string]any{
			"code": c.Code, "name": c.Name, "subject": c.Subject, "level": c.Level,
			"description": c.Description, "status": c.Status,
			"default_template_version_id": c.DefaultTemplateVersionID,
			"default_unit_price":          c.DefaultUnitPrice,
			"total_sessions":              c.TotalSessions, "duration_min": c.DurationMin,
			"updated_at": gorm.Expr("now()"),
		})
	return affected(res)
}

func (r *gormRepository) SetStatus(ctx context.Context, sc authctx.Scope, id uuid.UUID, status string) error {
	res := r.live(ctx, sc).
		Where("courses.id = ?", id).
		Updates(map[string]any{"status": status, "updated_at": gorm.Expr("now()")})
	return affected(res)
}

func (r *gormRepository) SoftDelete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.live(ctx, sc).
		Where("courses.id = ?", id).
		Updates(map[string]any{"deleted_at": gorm.Expr("now()"), "updated_at": gorm.Expr("now()")})
	return affected(res)
}

func (r *gormRepository) LiveClassCount(ctx context.Context, sc authctx.Scope, id uuid.UUID) (int64, error) {
	var n int64
	err := database.FromContext(ctx, r.db).
		Model(&classes.Class{}).
		Where("classes.center_id = ? AND classes.course_id = ? AND classes.deleted_at IS NULL", sc.CenterID, id).
		Count(&n).Error
	return n, err
}

// stageLinks joins the course's stage links with their live paths. The
// paths package owns those tables and imports nothing from here, so the
// join names them directly rather than through its models.
func (r *gormRepository) stageLinks(ctx context.Context, sc authctx.Scope, courseID uuid.UUID) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Table("path_stage_courses psc").
		Joins("JOIN path_stages ps ON ps.id = psc.stage_id AND ps.center_id = psc.center_id").
		Joins("JOIN learning_paths lp ON lp.id = ps.path_id AND lp.center_id = ps.center_id AND lp.deleted_at IS NULL").
		Where("psc.center_id = ? AND psc.course_id = ?", sc.CenterID, courseID)
}

func (r *gormRepository) PathCount(ctx context.Context, sc authctx.Scope, id uuid.UUID) (int64, error) {
	var n int64
	err := r.stageLinks(ctx, sc, id).Count(&n).Error
	return n, err
}

func (r *gormRepository) ListPaths(ctx context.Context, sc authctx.Scope, id uuid.UUID) ([]CoursePath, error) {
	var rows []CoursePath
	err := r.stageLinks(ctx, sc, id).
		Select("lp.id, lp.code, lp.name, lp.status, ps.id AS stage_id, ps.name AS stage_name, ps.position AS stage_position").
		Order("lp.name, lp.id, ps.position").
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) FindTemplateVersion(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (*TemplateVersionRef, error) {
	var ref TemplateVersionRef
	err := database.FromContext(ctx, r.db).
		Model(&library.Version{}).
		Select("program_template_versions.id, program_template_versions.template_id, program_template_versions.version_no, program_template_versions.status, t.code AS template_code, t.name AS template_name").
		Joins("JOIN program_templates t ON t.id = program_template_versions.template_id AND t.deleted_at IS NULL").
		Where("program_template_versions.center_id = ? AND program_template_versions.id = ?", sc.CenterID, versionID).
		Take(&ref).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &ref, nil
}

func (r *gormRepository) packs(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&TuitionPack{}).
		Where("course_tuition_packs.center_id = ?", sc.CenterID)
}

func (r *gormRepository) ListPacks(ctx context.Context, sc authctx.Scope, courseIDs []uuid.UUID) ([]TuitionPack, error) {
	if len(courseIDs) == 0 {
		return nil, nil
	}
	var rows []TuitionPack
	err := r.packs(ctx, sc).
		Where("course_id IN ?", courseIDs).
		Order("course_id, position").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) ReplacePacks(ctx context.Context, sc authctx.Scope, courseID uuid.UUID, rows []*TuitionPack) error {
	if err := r.packs(ctx, sc).Where("course_id = ?", courseID).Delete(&TuitionPack{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	for i, p := range rows {
		if p.ID == uuid.Nil {
			p.ID = id.New()
		}
		p.CourseID, p.CenterID, p.Position = courseID, sc.CenterID, i+1
	}
	return database.FromContext(ctx, r.db).Create(rows).Error
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

func notFoundOr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
