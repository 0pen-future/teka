package billing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// Service owns billing_periods lifecycle.
type Service struct {
	repo    Repository
	tx      database.TxManager
	pending PendingSource
	// enrollments is the sanctioned roster-membership check
	// (enrollments.Service.ActiveOn) plan 04's post-close reconciliation uses
	// for its one rare case: a session whose enrollment has no line yet on
	// the closed invoice. Nil-safe callers are unnecessary — router.go always
	// wires a real *enrollments.Service, and the two lightweight billing test
	// files that construct a Service directly never exercise
	// ReconcileSession.
	enrollments EnrollmentSource
	// now is overridden in tests so Close's "today" cutoff is deterministic;
	// production always gets time.Now.
	now func() time.Time
}

// NewService builds the billing service. pending is the narrow
// consumer-defined interface billing's period close and its future-sessions
// warning use to reuse sessions.Service's unconfirmed-sessions predicate —
// billing writes no session-scanning query of its own. enrollmentSource is
// the same narrow reuse for the roster-membership check plan 04's
// reconciliation needs.
func NewService(repo Repository, tx database.TxManager, pending PendingSource, enrollmentSource EnrollmentSource) *Service {
	return &Service{repo: repo, tx: tx, pending: pending, enrollments: enrollmentSource, now: time.Now}
}

// EnsurePeriod idempotently creates-or-gets the (teacher, year, month)
// period, computing period_start/period_end as the first and last calendar
// day of the month in the teacher's timezone. A concurrent duplicate insert —
// the unique index, not a pre-check, is what refuses it — resolves to the
// existing row rather than a 409, so two callers racing to open the same
// month converge on one period id. A new period always self-assigns to the
// caller: TeacherID/CenterID come from sc, never from a resolved target, so
// even a center owner creating a period only ever creates their own.
func (s *Service) EnsurePeriod(ctx context.Context, sc authctx.Scope, year, month int) (*Period, error) {
	// EnsurePeriodRequest's binding tags (min=2020,max=2100 / min=1,max=12)
	// already reject out-of-range values at the HTTP boundary; re-check here
	// so this exported method is safe to call directly (tests, future
	// callers) and the int16 conversion below never truncates.
	if year < 2020 || year > 2100 || month < 1 || month > 12 {
		return nil, apperror.Invalid("year must be 2020-2100 and month must be 1-12", nil)
	}

	loc, err := s.teacherLocation(ctx, sc.TeacherID)
	if err != nil {
		return nil, err
	}

	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, -1)
	// period_start/period_end are DATE columns; only the calendar day
	// resolved in the teacher's timezone is meaningful, so re-express at UTC
	// midnight the way the stored value compares.
	start = dateOnly(start)
	end = dateOnly(end)

	period := &Period{
		ID:          id.New(),
		TeacherID:   sc.TeacherID,
		CenterID:    sc.CenterID,
		Year:        int16(year),  // #nosec G115 -- bounds-checked above (2020-2100)
		Month:       int16(month), // #nosec G115 -- bounds-checked above (1-12)
		PeriodStart: start,
		PeriodEnd:   end,
		Status:      PeriodOpen,
	}
	err = s.repo.CreatePeriod(ctx, period)
	if errors.Is(err, ErrDuplicatePeriod) {
		existing, getErr := s.repo.GetPeriodByYearMonth(ctx, sc, int16(year), int16(month)) // #nosec G115 -- bounds-checked above
		if getErr != nil {
			return nil, apperror.Internal(getErr)
		}
		return existing, nil
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return period, nil
}

// ListPeriods returns a page of the tenant's billing periods.
func (s *Service) ListPeriods(ctx context.Context, sc authctx.Scope, p pagination.Params) ([]Period, int64, error) {
	return s.repo.ListPeriods(ctx, sc, p)
}

// GetPeriod returns one billing period.
func (s *Service) GetPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*Period, error) {
	p, err := s.repo.GetPeriod(ctx, sc, periodID)
	if errors.Is(err, ErrPeriodNotFound) {
		return nil, apperror.NotFound("billing period")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return p, nil
}

// Preview computes what closing this period would produce right now,
// without writing anything (R4's chốt sổ review screen).
func (s *Service) Preview(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*PreviewResponse, error) {
	compute, err := ComputePeriod(ctx, s.repo, sc, periodID)
	if errors.Is(err, ErrPeriodNotFound) {
		return nil, apperror.NotFound("billing period")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return buildPreviewResponse(compute, nil), nil
}

// Draft persists Preview's computation as invoices and invoice_lines with
// status='draft', so review adjustments (phase 4) have a row to attach to.
// Only allowed while the period is open; refuses to touch an invoice that
// has already moved past draft. Idempotent — running it twice on unchanged
// attendance produces the same rows, not duplicates.
func (s *Service) Draft(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*PreviewResponse, error) {
	period, err := s.repo.GetPeriod(ctx, sc, periodID)
	if errors.Is(err, ErrPeriodNotFound) {
		return nil, apperror.NotFound("billing period")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if period.Status != PeriodOpen {
		return nil, apperror.Conflict("period is closed")
	}

	compute, err := ComputePeriod(ctx, s.repo, sc, periodID)
	if err != nil {
		return nil, apperror.Internal(err)
	}

	// Every write below inherits the period's own anchors, never the acting
	// caller's — an owner drafting a member's period writes rows owned by
	// the member, not by the owner.
	periodScope := authctx.Scope{TeacherID: period.TeacherID, CenterID: period.CenterID}
	var invoiceIDByStudent map[uuid.UUID]uuid.UUID
	err = s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		var txErr error
		invoiceIDByStudent, txErr = DraftPeriod(txCtx, s.repo, periodScope, periodID, compute)
		return txErr
	})
	if errors.Is(err, ErrInvoiceNotDraft) {
		return nil, apperror.Conflict("an invoice for this period has already been issued")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}

	return buildPreviewResponse(compute, invoiceIDByStudent), nil
}

// teacherLocation resolves the teacher's IANA timezone, falling back to UTC
// if the stored value fails to load. teachers.UpdateProfile already validates
// IANA zones on write, so a load failure here would only mean stale seed
// data, not a request the caller can fix.
func (s *Service) teacherLocation(ctx context.Context, teacherID uuid.UUID) (*time.Location, error) {
	tz, err := s.repo.TeacherTimezone(ctx, teacherID)
	if errors.Is(err, ErrTeacherNotFound) {
		return nil, apperror.NotFound("teacher")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC, nil
	}
	return loc, nil
}

// dateOnly re-expresses an instant's calendar day at UTC midnight, matching
// how DATE columns are stored and compared.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
