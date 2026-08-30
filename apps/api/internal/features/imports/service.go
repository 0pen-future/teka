package imports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

// maxReportedErrors caps the error list in a 422. A garbage workbook can
// produce thousands; an operator cannot act on more than a screenful, and the
// omitted count tells them the list was cut.
const maxReportedErrors = 100

// MemberDirectory is the slice of center membership this feature consumes
// (consumer-defined interface; implemented by *centers.Service). Resolving a
// teacher phone through a directory derived from the caller's own scope is
// what keeps the import inside one center — there is deliberately no
// by-id or global-phone lookup on this interface. CenterOwner resolves the
// caller's center owner, the anchor every imported contact and student is
// written under regardless of who runs the import.
type MemberDirectory interface {
	MemberIDsByPhone(ctx context.Context, scope authctx.Scope) (map[string]uuid.UUID, error)
	CenterOwner(ctx context.Context, teacherID uuid.UUID) (ownerID uuid.UUID, isOwner bool, err error)
}

// The four interfaces below are the slices of the roster features this one
// drives (consumer-defined, implemented by their *Service types). Every method
// takes an authctx.Scope, so each call stays inside the same scoped() filter
// the feature applies to its own HTTP traffic — this feature reaches no table
// it does not own except through the service that owns it.

// ClassWriter creates classes and their weekly slots, and answers whether
// either already exists.
type ClassWriter interface {
	FindActiveByName(ctx context.Context, sc authctx.Scope, name string) (*classes.Class, bool, error)
	ScheduleExists(ctx context.Context, sc authctx.Scope, classID uuid.UUID, weekday int16, startTime classes.TimeOfDay, effectiveFrom time.Time) (bool, error)
	Create(ctx context.Context, sc authctx.Scope, req classes.CreateClassRequest) (*classes.Class, error)
	AddSchedule(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req classes.ScheduleRequest) (*classes.Schedule, error)
}

// ContactWriter creates parent contacts and resolves them by phone.
type ContactWriter interface {
	FindIDByPhone(ctx context.Context, sc authctx.Scope, phone string) (uuid.UUID, bool, error)
	Create(ctx context.Context, sc authctx.Scope, req contacts.CreateRequest) (*contacts.Row, error)
}

// StudentWriter creates students and resolves them by contact, name and note.
type StudentWriter interface {
	FindIDByName(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, fullName string, note *string) (uuid.UUID, bool, error)
	Create(ctx context.Context, sc authctx.Scope, req students.CreateRequest) (*students.Row, error)
}

// EnrollmentWriter creates enrollments and resolves an existing one — open or
// already ended.
type EnrollmentWriter interface {
	FindByStudentAndClass(ctx context.Context, sc authctx.Scope, studentID, classID uuid.UUID) (*enrollments.Enrollment, bool, error)
	Create(ctx context.Context, sc authctx.Scope, req enrollments.CreateRequest) (*enrollments.Row, error)
}

// Service turns an uploaded workbook into roster rows. It owns no tables of
// its own: it resolves the workbook against the caller's center and then
// drives the classes, contacts, students and enrollments services.
type Service struct {
	members     MemberDirectory
	classes     ClassWriter
	contacts    ContactWriter
	students    StudentWriter
	enrollments EnrollmentWriter
	locker      Locker
	tx          database.TxManager
}

// NewService builds the imports service.
func NewService(
	members MemberDirectory,
	classSvc ClassWriter,
	contactSvc ContactWriter,
	studentSvc StudentWriter,
	enrollmentSvc EnrollmentWriter,
	locker Locker,
	tx database.TxManager,
) *Service {
	return &Service{
		members:     members,
		classes:     classSvc,
		contacts:    contactSvc,
		students:    studentSvc,
		enrollments: enrollmentSvc,
		locker:      locker,
		tx:          tx,
	}
}

// Template returns the blank workbook the operator fills in.
func (s *Service) Template(_ context.Context, scope authctx.Scope) ([]byte, error) {
	if err := requireImportsRun(scope); err != nil {
		return nil, err
	}
	b, err := BuildTemplate()
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return b, nil
}

// Import parses, resolves and cross-checks a workbook. With dryRun the result
// is a report of what would happen and nothing is written; the check runs the
// same resolution as the real pass, so a clean dry run cannot be followed by a
// resolution failure.
//
// Any invalid row rejects the whole file: a partially imported roster is worse
// than none, because the operator cannot tell which half landed.
func (s *Service) Import(ctx context.Context, scope authctx.Scope, file []byte, dryRun bool) (*Report, error) {
	if err := requireImportsRun(scope); err != nil {
		return nil, err
	}

	// Parsing happens after the owner gate so the endpoint is not a workbook
	// parser a member can drive.
	wb, rowErrs, err := ParseWorkbook(file)
	if err != nil {
		var fe *FileError
		if errors.As(err, &fe) {
			return nil, apperror.BadRequest(fe.Message)
		}
		return nil, apperror.Internal(err)
	}

	dir, err := s.members.MemberIDsByPhone(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Contacts and students are center data anchored on the owner, so the
	// import resolves the owner up front and writes them under a server-side
	// owner scope — a granted member's run produces exactly the rows the
	// owner's would. IsOwner true is what lets those writes through the
	// owner-only service gates and widens their dedupe lookups center-wide.
	ownerID := scope.TeacherID
	if !scope.IsOwner {
		ownerID, _, err = s.members.CenterOwner(ctx, scope.TeacherID)
		if err != nil {
			return nil, err
		}
	}
	ownerAnchor := authctx.Scope{TeacherID: ownerID, CenterID: scope.CenterID, IsOwner: true}

	plan, resolveErrs := resolve(wb, dir, ownerID)
	rowErrs = append(rowErrs, resolveErrs...)
	if len(rowErrs) > 0 {
		return nil, rowErrorsErr(rowErrs)
	}

	// A workbook that parses cleanly but carries no class and no student is a
	// whole-file mistake, not a row defect — most often data typed on the
	// example row (skipped by position) or an untouched template. Reject before
	// opening the transaction so neither the check nor the commit takes the
	// center lock for nothing.
	if len(plan.classes) == 0 && len(plan.students) == 0 {
		return nil, emptyFileErr()
	}

	rep := &Report{}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Three of the five natural keys have no unique index behind them, so
		// their pre-checks are only sound while nothing else writes this
		// center. The lock is taken for the dry run too: a check whose answer
		// could change under a concurrent import is not a check.
		locked, err := s.locker.TryLockCenter(ctx, scope.CenterID)
		if err != nil {
			return err
		}
		if !locked {
			return apperror.Conflict("một lượt import khác của trung tâm đang chạy; thử lại sau")
		}
		if err := s.locker.SetStatementTimeout(ctx); err != nil {
			return err
		}
		applyErrs, err := s.apply(ctx, ownerAnchor, plan, dryRun, rep)
		if err != nil {
			return err
		}
		if len(applyErrs) > 0 {
			// Roll back rather than commit a partial roster: an operator who
			// cannot tell which half landed is worse off than one who has to
			// fix the file and retry.
			return rowErrorsErr(applyErrs)
		}
		if dryRun {
			// Nothing was written, but the lookups above ran on the same
			// snapshot the commit would use. Roll back to release the lock
			// immediately instead of holding it until the function returns.
			return errDryRunRollback
		}
		return nil
	})
	switch {
	case errors.Is(err, errDryRunRollback):
	case err != nil:
		return nil, err
	}
	rep.Committed = !dryRun
	return rep, nil
}

// errDryRunRollback unwinds the transaction after a successful dry run. The
// dry run performs every lookup and no write, so there is nothing to keep —
// rolling back releases the center lock at once.
var errDryRunRollback = errors.New("imports: dry run complete")

// requireImportsRun gates every import route: assigning a class to a teacher
// is center administration, so members import only when granted imports.run.
func requireImportsRun(scope authctx.Scope) error {
	if !scope.Has(authctx.PermImportsRun) {
		return apperror.Forbidden("bạn không có quyền import dữ liệu")
	}
	return nil
}

// rowErrorsErr packages row defects as a 422 whose details half carries the
// list. Fields cannot express it: the operator needs sheet, line and column
// per defect, not one message per field name.
func rowErrorsErr(errs []RowError) error {
	payload := ErrorsPayload{Errors: errs}
	if len(errs) > maxReportedErrors {
		payload.Errors = errs[:maxReportedErrors]
		payload.Truncated = len(errs) - maxReportedErrors
	}
	return &RowErrorsError{
		AppError: apperror.Invalid("file có dòng không hợp lệ", nil),
		Payload:  payload,
	}
}

// RowErrorsError carries the row-defect list from the service to the handler,
// which unwraps it into the envelope's details field.
type RowErrorsError struct {
	*apperror.AppError
	Payload ErrorsPayload
}

// Unwrap exposes the embedded AppError to errors.As. Without it the promoted
// AppError.Unwrap would return its nil cause, the chain would end, and
// apperror.From would fall back to a 500 for what is a 422.
func (e *RowErrorsError) Unwrap() error { return e.AppError }
