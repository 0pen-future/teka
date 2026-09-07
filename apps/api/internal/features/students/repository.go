package students

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classscope"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows the student list. ClassID filters through open
// enrollments — the attendance screen's view of a class roster. Unenrolled
// keeps only students with no open enrollment in any class — the roster's
// "Chưa ghi danh" tab.
type ListFilter struct {
	Query      string
	ContactID  uuid.UUID
	ClassID    uuid.UUID
	Unenrolled bool
}

// Row is a student joined with its contact's name and phone, so the roster
// screen needs no second call. ContactPhone is a pointer because the service
// nulls it for callers outside the phone rule; PhoneVisible is the repo-derived
// row grant (active hoc_vu stint on a class with an active enrollment) that
// Scope.PhoneVisible combines with the owner/oversight bypass.
type Row struct {
	Student      `gorm:"embedded"`
	ContactName  string
	ContactPhone *string
	PhoneVisible bool
}

// Repository is the persistence contract for students; the service depends on
// this interface, tests supply a fake.
type Repository interface {
	Create(ctx context.Context, s *Student) error
	GetByID(ctx context.Context, sc authctx.Scope, studentID uuid.UUID) (*Row, error)
	List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Row, int64, error)
	Update(ctx context.Context, sc authctx.Scope, s *Student) error
	// ContactExists reports whether the contact is live under this scope —
	// the clean-422 check in front of the composite-FK safety net.
	ContactExists(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) (bool, error)
	// AnonymizeAndDelete scrubs PII and hides the row in one scoped UPDATE:
	// full_name becomes the placeholder, display_note goes NULL, and both
	// anonymized_at and deleted_at are stamped. The row itself survives so
	// financial foreign keys keep holding. Runs on the context transaction so
	// the caller can close enrollments atomically alongside it.
	AnonymizeAndDelete(ctx context.Context, sc authctx.Scope, studentID uuid.UUID, placeholder string) error
	// FindIDByName resolves a live student by contact, exact name, and note.
	// note is a pointer because display_note is NULL when unset.
	FindIDByName(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, fullName string, note *string) (uuid.UUID, bool, error)
	// GetByIDAnchored reads a student bound unconditionally to a — the roster
	// import's read-back after CreateAnchored, with no caller scope to widen
	// or narrow it.
	GetByIDAnchored(ctx context.Context, a authctx.Anchor, studentID uuid.UUID) (*Row, error)
	// ContactExistsAnchored reports whether the contact anchors exactly to a.
	// The roster import's clean-422 check: a student anchored on a must
	// reference a contact anchored on that same party, not merely one the
	// importing member happens to see.
	ContactExistsAnchored(ctx context.Context, a authctx.Anchor, contactID uuid.UUID) (bool, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// centerScoped returns a READ query bound to one center only, no teacher_id
// branch. Students anchor to the center's owner regardless of who created
// them, so dedupe-by-name (FindIDByName, the bulk-import matcher) must
// resolve identically for the owner and for a member running an import on
// the owner's behalf — narrowing to the caller would miss existing students
// and duplicate the roster on re-import.
func (r *gormRepository) centerScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).Where("students.center_id = ?", sc.CenterID)
}

// anchored returns a query bound unconditionally to a's two columns — no
// scope branching, since the caller (the roster import's write path) has
// already resolved whose rows to touch. Backs GetByIDAnchored.
func (r *gormRepository) anchored(ctx context.Context, a authctx.Anchor) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Where("students.center_id = ? AND students.teacher_id = ?", a.CenterID, a.TeacherID)
}

// writeScoped is the scope for mutating queries: own rows only, owner
// excepted. students.view_all is a visibility key — it widens scoped and
// readScoped, never writes — because member writes were own-rows-only before
// the catalog existed, so a key widening writes would be an escalation. A
// mutation matching zero rows here is reported as not-found by the callers,
// masking rows the caller can read but not write.
func (r *gormRepository) writeScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("students.center_id = ?", sc.CenterID)
	if !sc.WriteWide() {
		q = q.Where("students.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// readScoped additionally lets a member read students enrolled in classes
// they hold a class_staff stint on — the current teacher after a handoff
// (rows never move), and any other staff assignment, ended ones included.
// Reads only: editing or anonymizing a student stays with its creator or the
// owner, so write paths keep scoped.
func (r *gormRepository) readScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("students.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermStudentsViewAll) {
		frag, _ := classscope.ReadExistsViaEnrollment("students.id")
		q = q.Where("(students.teacher_id = ? OR "+frag+")",
			sc.TeacherID, sc.TeacherID, sc.CenterID)
	}
	return q
}

// withContact joins the owning contact and selects its name and phone
// alongside the student columns, plus the phone_visible derived column the
// service needs to apply the phone rule. The EXISTS runs for every caller —
// owner included — because whether the CALLER may bypass it is the service's
// call, not this query's. Takes the two identity columns directly, not a
// Scope, so an anchor-only caller (GetByIDAnchored) can use it too.
func withContact(q *gorm.DB, teacherID, centerID uuid.UUID) *gorm.DB {
	frag, _ := classscope.PhoneVisibleViaStudent("students.id")
	return q.
		Joins("JOIN contacts ON contacts.id = students.contact_id AND contacts.center_id = students.center_id").
		Select("students.*, contacts.full_name AS contact_name, contacts.phone AS contact_phone, "+
			frag+" AS phone_visible", teacherID, centerID)
}

func (r *gormRepository) Create(ctx context.Context, s *Student) error {
	err := database.FromContext(ctx, r.db).Create(s).Error
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		// The composite FK (contact_id, center_id) refused the insert — the
		// contact is not this center's. Reached only when the pre-check raced.
		return ErrContactNotOwned
	}
	return err
}

func (r *gormRepository) GetByID(ctx context.Context, sc authctx.Scope, studentID uuid.UUID) (*Row, error) {
	var row Row
	err := withContact(r.readScoped(ctx, sc).Model(&Student{}), sc.TeacherID, sc.CenterID).
		Where("students.id = ?", studentID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// GetByIDAnchored reads a student bound unconditionally to a. It backs
// CreateAnchored's read-back: the roster import has no caller scope to
// consult for the phone rule, so the returned row's phone_visible column
// reflects only a's own hoc_vu-stint reach — the service still nulls the
// phone unless a caller scope grants it, and the import path never does.
func (r *gormRepository) GetByIDAnchored(ctx context.Context, a authctx.Anchor, studentID uuid.UUID) (*Row, error) {
	var row Row
	err := withContact(r.anchored(ctx, a).Model(&Student{}), a.TeacherID, a.CenterID).
		Where("students.id = ?", studentID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Row, int64, error) {
	q := r.readScoped(ctx, sc).Model(&Student{})
	if filter.Query != "" {
		q = q.Where("students.full_name ILIKE ?", "%"+filter.Query+"%")
	}
	if filter.ContactID != uuid.Nil {
		q = q.Where("students.contact_id = ?", filter.ContactID)
	}
	if filter.ClassID != uuid.Nil {
		// Open enrollment in the class; the partial unique index guarantees at
		// most one per student, so the join cannot duplicate rows.
		q = q.Joins(
			"JOIN enrollments ON enrollments.student_id = students.id"+
				" AND enrollments.class_id = ? AND enrollments.ended_on IS NULL AND enrollments.deleted_at IS NULL",
			filter.ClassID)
	}
	if filter.Unenrolled {
		q = q.Where("NOT EXISTS (SELECT 1 FROM enrollments" +
			" WHERE enrollments.student_id = students.id" +
			" AND enrollments.ended_on IS NULL AND enrollments.deleted_at IS NULL)")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Row
	if err := withContact(q, sc.TeacherID, sc.CenterID).Scopes(p.Scope).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Update writes through the write scope, not the widened read scope: a
// member may read a student via a class-staff stint yet must not edit it, so
// a row outside their own is masked as not-found — the same shape
// AnonymizeAndDelete answers with.
func (r *gormRepository) Update(ctx context.Context, sc authctx.Scope, s *Student) error {
	res := r.writeScoped(ctx, sc).
		Model(&Student{}).
		Where("students.id = ?", s.ID).
		Updates(map[string]any{
			"contact_id":   s.ContactID,
			"full_name":    s.FullName,
			"display_note": s.DisplayNote,
		})
	if errors.Is(res.Error, gorm.ErrForeignKeyViolated) {
		return ErrContactNotOwned
	}
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) ContactExists(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) (bool, error) {
	var n int64
	q := database.FromContext(ctx, r.db).
		Table("contacts").
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", contactID, sc.CenterID)
	// Keys on contacts.view_all, not students.view_all: this probes which
	// CONTACTS the caller may anchor a student to, so it follows contact
	// visibility, not student visibility. Deliberately narrower than the
	// contacts read predicate: no oversight or hoc_vu-stint arm, and the
	// teacher_id arm is dead for non-owners (contacts anchor to the owner) —
	// anchoring students to foreign contacts stays an explicit-grant action.
	q = r.readNarrowContacts(q, sc)
	err := q.Count(&n).Error
	return n > 0, err
}

// readNarrowContacts narrows a contacts query to the caller's own rows unless
// they see contacts center-wide. A read narrowing: it decides which contacts
// are visible to anchor a student to, not who may write the student.
func (r *gormRepository) readNarrowContacts(q *gorm.DB, sc authctx.Scope) *gorm.DB {
	if !sc.CenterWideFor(authctx.PermContactsViewAll) {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	return q
}

// ContactExistsAnchored reports whether the contact anchors exactly to a: the
// roster import's clean-422 check ahead of CreateAnchored. Unlike
// ContactExists (which follows the caller's own visibility, view_all
// included), this always requires an exact match — a student anchored on a
// must reference a contact anchored on that same party, never merely one a
// grant happens to expose to the importing member.
func (r *gormRepository) ContactExistsAnchored(ctx context.Context, a authctx.Anchor, contactID uuid.UUID) (bool, error) {
	var n int64
	err := database.FromContext(ctx, r.db).
		Table("contacts").
		Where("id = ? AND center_id = ? AND teacher_id = ? AND deleted_at IS NULL", contactID, a.CenterID, a.TeacherID).
		Count(&n).Error
	return n > 0, err
}

func (r *gormRepository) AnonymizeAndDelete(ctx context.Context, sc authctx.Scope, studentID uuid.UUID, placeholder string) error {
	res := r.writeScoped(ctx, sc).
		Model(&Student{}).
		Where("students.id = ?", studentID).
		Updates(map[string]any{
			"full_name":     placeholder,
			"display_note":  nil,
			"anonymized_at": gorm.Expr("now()"),
			"deleted_at":    gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// FindIDByName resolves a student by their identity within one contact: the
// exact name plus the note that distinguishes same-named siblings, center-wide
// — see centerScoped.
//
// The note comparison is IS NOT DISTINCT FROM, not =. display_note is NULL
// when unset (notePtr), and `display_note = ”` never matches NULL in SQL —
// with an equality test a bulk importer would miss every note-less student,
// which is nearly all of them, and duplicate the entire roster on re-import.
func (r *gormRepository) FindIDByName(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, fullName string, note *string) (uuid.UUID, bool, error) {
	// Scanning into a bare uuid.UUID would skip its sql.Scanner and hit
	// GORM's element-wise array path ([16]byte); the id has to land in a
	// struct field.
	var row struct{ ID uuid.UUID }
	err := r.centerScoped(ctx, sc).Model(&Student{}).
		Where("students.contact_id = ? AND students.full_name = ?", contactID, fullName).
		Where("students.display_note IS NOT DISTINCT FROM ?", note).
		Limit(1).
		Select("students.id").
		Scan(&row).Error
	if err != nil {
		return uuid.Nil, false, err
	}
	return row.ID, row.ID != uuid.Nil, nil
}
