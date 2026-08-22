package billing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// EnrollmentSource is the slice of the enrollments feature billing's
// post-close reconciliation needs: the roster active on a class's session
// date, the exact sanctioned membership check attendance.Confirm itself
// uses. *enrollments.Service satisfies this. Declared here — a
// consumer-defined interface, the same pattern AttendanceSource and
// PendingSource use — so a reconciliation never hand-rolls its own
// started_on/ended_on comparison.
type EnrollmentSource interface {
	ActiveOn(ctx context.Context, sc authctx.Scope, classID uuid.UUID, on time.Time) ([]enrollments.Enrollment, error)
}

// minAdjustmentReasonLen/maxAdjustmentReasonLen mirror AdjustmentRequest's
// binding tags. AddAdjustment re-checks them explicitly (defense in depth
// for a caller that bypasses Gin binding) before any adjustment ever reaches
// the invoice_adjustments CHECK constraint.
const (
	minAdjustmentReasonLen = 3
	maxAdjustmentReasonLen = 500
)

// AddAdjustment posts one manual, signed correction to an invoice (R4):
// negative reduces the bill, positive increases it. Allowed while the
// invoice is draft, issued, or partially_paid; refused with 409 on void or
// paid. Recomputes adjustment_total, total_due, and status in the same
// transaction the adjustment row is created in.
func (s *Service) AddAdjustment(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID, amount int64, reason string) (*AdjustmentResponse, *InvoiceResponse, error) {
	if amount == 0 {
		return nil, nil, apperror.Invalid("validation failed", map[string]string{"amount": "must not be zero"})
	}
	if err := validateAdjustmentReason(reason); err != nil {
		return nil, nil, err
	}

	inv, err := s.repo.GetInvoice(ctx, sc, invoiceID)
	if errors.Is(err, ErrInvoiceNotFound) {
		return nil, nil, apperror.NotFound("invoice")
	}
	if err != nil {
		return nil, nil, apperror.Internal(err)
	}
	switch inv.Status {
	case InvoiceVoid:
		return nil, nil, apperror.Conflict("invoice is void; adjustments cannot be posted")
	case InvoicePaid:
		return nil, nil, apperror.Conflict("invoice is fully paid; post the adjustment on the next period instead")
	}

	// The adjustment inherits the invoice's own anchors, never the acting
	// caller's — an owner adjusting a member's invoice writes a row owned by
	// the member, not by the owner.
	adj := &InvoiceAdjustment{
		ID:        id.New(),
		TeacherID: inv.TeacherID,
		CenterID:  inv.CenterID,
		InvoiceID: invoiceID,
		Amount:    amount,
		Reason:    reason,
	}
	err = s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.CreateAdjustment(txCtx, adj); err != nil {
			return err
		}
		return s.repo.RecalcInvoiceTotals(txCtx, sc, invoiceID)
	})
	if err != nil {
		return nil, nil, apperror.Internal(err)
	}

	updated, err := s.repo.GetInvoice(ctx, sc, invoiceID)
	if err != nil {
		return nil, nil, apperror.Internal(err)
	}
	adjResp := FromAdjustmentModel(adj)
	invResp := FromInvoiceModel(updated)
	return &adjResp, &invResp, nil
}

// ListAdjustments returns invoiceID's adjustment audit trail (R4), 404 for a
// missing or cross-tenant invoice.
func (s *Service) ListAdjustments(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) ([]AdjustmentResponse, error) {
	_, err := s.repo.GetInvoice(ctx, sc, invoiceID)
	if errors.Is(err, ErrInvoiceNotFound) {
		return nil, apperror.NotFound("invoice")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	rows, err := s.repo.ListAdjustments(ctx, sc, invoiceID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return FromAdjustmentModels(rows), nil
}

func validateAdjustmentReason(reason string) error {
	n := len([]rune(reason))
	if n < minAdjustmentReasonLen || n > maxAdjustmentReasonLen {
		return apperror.Invalid("validation failed", map[string]string{
			"reason": fmt.Sprintf("must be between %d and %d characters", minAdjustmentReasonLen, maxAdjustmentReasonLen),
		})
	}
	return nil
}

// deriveInvoiceStatus is billing's own narrow stand-in for the status
// derivation plan 05 (payments) will own canonically: it moves an invoice
// among issued, partially_paid, and paid based on paid_amount vs total_due,
// and never touches draft or void — those stay owned by Draft and
// VoidInvoice respectively. RecalcInvoiceTotals mirrors this exact rule
// inline in SQL (not by calling this function), so the whole
// adjustment_total/total_due/status recompute commits as a single atomic
// UPDATE; this Go copy exists so the rule is unit-testable and documented in
// one place plan 05 can replace outright.
func deriveInvoiceStatus(currentStatus string, paidAmount, totalDue int64) string {
	if currentStatus == InvoiceDraft || currentStatus == InvoiceVoid {
		return currentStatus
	}
	switch {
	case paidAmount <= 0:
		return InvoiceIssued
	case paidAmount < totalDue:
		return InvoicePartiallyPaid
	default:
		return InvoicePaid
	}
}

// resolveDelta is the pure arithmetic core of a post-close reconciliation:
// how much a closed invoice's charge would differ today from what it
// actually billed, net of any carry already posted for the same closed
// period (already_adj). hasInvoice=false (the student has no non-void
// invoice in the closed period — Architecture: skip) always yields
// (0, false) without looking at the other arguments. Zero delta — including
// the everyday case of an edit that never changed a billable count, and the
// case of a second edit exactly cancelling a first one — also yields
// shouldPost=false.
func resolveDelta(hasInvoice bool, liveCharge, currentCharge, alreadyAdjusted int64) (delta int64, shouldPost bool) {
	if !hasInvoice {
		return 0, false
	}
	d := liveCharge - currentCharge - alreadyAdjusted
	return d, d != 0
}

// nextCalendarMonth returns the (year, month) immediately after the given
// one, rolling December into January of the next year.
func nextCalendarMonth(year, month int) (int, int) {
	month++
	if month > 12 {
		return year + 1, 1
	}
	return year, month
}

// adjustmentReason renders the reconciliation reason text the teacher sees
// on the carried adjustment, per Architecture's template.
func adjustmentReason(sessionDate time.Time, className string, period *Period) string {
	return fmt.Sprintf("Điều chỉnh do sửa điểm danh buổi %s lớp %s của kỳ %02d/%d",
		sessionDate.Format("02/01/2006"), className, period.Month, period.Year)
}

// ReconcileSession implements attendance.BillingReconciler: after a
// committed attendance change to sessionID, it checks whether that session's
// date falls inside an already-closed billing period and, if so, carries
// each affected student's resulting money delta onto the next open period's
// invoice (PRD R4, D7). Returns an empty Reconciliation (not an error) when
// the session's date is still inside an open period, has no closed period at
// all, or has no attendance recorded yet.
func (s *Service) ReconcileSession(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (attendance.Reconciliation, error) {
	classID, className, sessionDate, sessionScope, err := s.repo.SessionMeta(ctx, sc, sessionID)
	if errors.Is(err, ErrSessionNotFound) {
		return attendance.Reconciliation{}, nil
	}
	if err != nil {
		return attendance.Reconciliation{}, apperror.From(err)
	}

	// Everything downstream of this point uses the session's own derived
	// scope, never the caller's — attendance confirmation triggers this from
	// a variety of callers, and a date-range/no-id-anchor query under an
	// owner's raw sc would otherwise widen it to the whole center.
	period, err := s.repo.PeriodContainingDate(ctx, sessionScope, sessionDate)
	if err != nil {
		return attendance.Reconciliation{}, apperror.From(err)
	}
	if period == nil {
		return attendance.Reconciliation{}, nil
	}

	records, err := s.repo.SessionAttendance(ctx, sessionScope, sessionID)
	if err != nil {
		return attendance.Reconciliation{}, apperror.From(err)
	}
	if len(records) == 0 {
		return attendance.Reconciliation{}, nil
	}

	// One session belongs to one class; each attendance row already carries
	// its own enrollment_id, so a plain de-dup by student is enough to
	// enumerate every affected student without a second roster lookup.
	type affected struct {
		enrollmentID uuid.UUID
	}
	byStudent := make(map[uuid.UUID]affected, len(records))
	for _, rec := range records {
		byStudent[rec.StudentID] = affected{enrollmentID: rec.EnrollmentID}
	}
	// Lock the closed invoices in a stable (sorted) student order so two
	// concurrent reconciliations that touch an overlapping student set can
	// never acquire the same two row locks in opposite order and deadlock.
	studentIDs := make([]uuid.UUID, 0, len(byStudent))
	for studentID := range byStudent {
		studentIDs = append(studentIDs, studentID)
	}
	sort.Slice(studentIDs, func(i, j int) bool {
		return studentIDs[i].String() < studentIDs[j].String()
	})

	// period may belong to a different teacher than the session did if the
	// class moved teachers between the session's date and now — reconciling
	// through the PERIOD's own anchors keeps every downstream write correct
	// either way.
	periodScope := authctx.Scope{TeacherID: period.TeacherID, CenterID: period.CenterID}

	var result attendance.Reconciliation
	err = s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		for _, studentID := range studentIDs {
			a := byStudent[studentID]
			entry, rcErr := s.reconcileStudent(txCtx, periodScope, period, sessionID, classID, className, sessionDate, studentID, a.enrollmentID)
			if rcErr != nil {
				return rcErr
			}
			if entry != nil {
				result.Adjustments = append(result.Adjustments, *entry)
			}
		}
		return nil
	})
	if err != nil {
		return attendance.Reconciliation{}, apperror.From(err)
	}
	return result, nil
}

// reconcileStudent computes and, if non-zero, posts one student's carried
// delta for a session edited after its period closed. Returns a nil entry
// and writes nothing when the student has no non-void invoice in the closed
// period, or the delta comes out exactly zero.
func (s *Service) reconcileStudent(
	ctx context.Context, periodScope authctx.Scope, period *Period,
	sessionID, sessionClassID uuid.UUID, sessionClassName string, sessionDate time.Time,
	studentID, sessionEnrollmentID uuid.UUID,
) (*attendance.ReconciliationEntry, error) {
	invoices, err := s.repo.ListInvoices(ctx, periodScope, period.ID)
	if err != nil {
		return nil, err
	}
	var inv *Invoice
	for i := range invoices {
		if invoices[i].StudentID == studentID {
			inv = &invoices[i]
			break
		}
	}
	if inv == nil || inv.Status == InvoiceVoid {
		return nil, nil
	}

	// Serialise concurrent reconciliations of this same student's closed
	// period: lock the closed invoice row before reading already_adj below, so
	// a second edit's transaction blocks here until the first commits and then
	// reads an already_adj that already includes the first carry — otherwise
	// both would read already_adj=0 and post the same delta twice, over-billing
	// the parent. The lock is held for the rest of the enclosing tx.
	locked, err := s.repo.LockInvoice(ctx, periodScope, inv.ID)
	if err != nil {
		return nil, err
	}
	if locked.Status == InvoiceVoid {
		// Voided concurrently between the unlocked ListInvoices read above and
		// acquiring the lock — nothing to carry.
		return nil, nil
	}
	inv = locked

	_, lines, err := s.repo.GetInvoiceWithLines(ctx, periodScope, inv.ID)
	if err != nil {
		return nil, err
	}

	enrollmentIDs := make([]uuid.UUID, 0, len(lines)+1)
	hasSessionLine := false
	for _, l := range lines {
		enrollmentIDs = append(enrollmentIDs, l.EnrollmentID)
		if l.EnrollmentID == sessionEnrollmentID {
			hasSessionLine = true
		}
	}
	if !hasSessionLine {
		enrollmentIDs = append(enrollmentIDs, sessionEnrollmentID)
	}

	liveCounts, err := s.repo.LiveBillableCounts(ctx, enrollmentIDs, period)
	if err != nil {
		return nil, err
	}

	var liveCharge int64
	for _, l := range lines {
		liveCharge += int64(liveCounts[l.EnrollmentID]) * l.UnitPrice
	}
	if !hasSessionLine {
		// Rare: the edited session's own enrollment has no line on the
		// closed invoice at all — its first-ever confirmed attendance in
		// this period happened after close. Roster membership is confirmed
		// through the sanctioned enrollments.ActiveOn call, never a
		// hand-written date comparison.
		roster, rosterErr := s.enrollments.ActiveOn(ctx, periodScope, sessionClassID, sessionDate)
		if rosterErr != nil {
			return nil, rosterErr
		}
		for _, e := range roster {
			if e.ID == sessionEnrollmentID && e.StudentID == studentID {
				liveCharge += int64(liveCounts[sessionEnrollmentID]) * e.UnitPrice
				break
			}
		}
	}

	alreadyAdj, err := s.repo.AdjustmentsBySourcePeriod(ctx, periodScope, studentID, period.ID)
	if err != nil {
		return nil, err
	}

	delta, shouldPost := resolveDelta(true, liveCharge, inv.CurrentCharge, alreadyAdj)
	if !shouldPost {
		return nil, nil
	}

	targetPeriod, targetInvoiceID, err := s.ensureAdjustmentTarget(ctx, periodScope, period, studentID)
	if err != nil {
		return nil, err
	}

	adj := &InvoiceAdjustment{
		ID:              id.New(),
		TeacherID:       periodScope.TeacherID,
		CenterID:        periodScope.CenterID,
		InvoiceID:       targetInvoiceID,
		Amount:          delta,
		Reason:          adjustmentReason(sessionDate, sessionClassName, period),
		SourceSessionID: &sessionID,
	}
	if err := s.repo.CreateAdjustment(ctx, adj); err != nil {
		return nil, err
	}
	if err := s.repo.RecalcInvoiceTotals(ctx, periodScope, targetInvoiceID); err != nil {
		return nil, err
	}

	return &attendance.ReconciliationEntry{
		StudentID: studentID,
		PeriodID:  targetPeriod.ID,
		InvoiceID: targetInvoiceID,
		Amount:    delta,
	}, nil
}

// ensureAdjustmentTarget resolves (creating if necessary) the period and
// invoice a reconciliation delta lands on: the earliest already-open period
// after the closed period p, or — if none exists yet — the current calendar
// month (rolling forward past any month that turns out already closed),
// with a draft invoice for the student ensured on it.
func (s *Service) ensureAdjustmentTarget(ctx context.Context, periodScope authctx.Scope, p *Period, studentID uuid.UUID) (*Period, uuid.UUID, error) {
	target, err := s.resolveTargetPeriod(ctx, periodScope, p)
	if err != nil {
		return nil, uuid.UUID{}, err
	}

	existing, err := s.repo.ListInvoices(ctx, periodScope, target.ID)
	if err != nil {
		return nil, uuid.UUID{}, err
	}
	for _, inv := range existing {
		if inv.StudentID == studentID {
			return target, inv.ID, nil
		}
	}

	contactID, studentName, contactName, err := s.repo.StudentSnapshot(ctx, periodScope, studentID)
	if err != nil {
		return nil, uuid.UUID{}, err
	}
	// The new draft invoice inherits the target period's own anchors, never
	// the acting caller's — mirrors DraftPeriod's rule.
	inv := &Invoice{
		ID:          id.New(),
		TeacherID:   periodScope.TeacherID,
		CenterID:    periodScope.CenterID,
		PeriodID:    target.ID,
		StudentID:   studentID,
		ContactID:   contactID,
		StudentName: studentName,
		ContactName: contactName,
		Status:      InvoiceDraft,
	}
	if err := s.repo.UpsertInvoice(ctx, inv); err != nil {
		if errors.Is(err, ErrInvoiceNotDraft) {
			// Lost a race with a concurrent draft/close on the same
			// brand-new target period between the existence check above and
			// this insert; re-resolve the now-existing row instead of
			// failing the whole reconciliation.
			refreshed, listErr := s.repo.ListInvoices(ctx, periodScope, target.ID)
			if listErr != nil {
				return nil, uuid.UUID{}, listErr
			}
			for _, r := range refreshed {
				if r.StudentID == studentID {
					return target, r.ID, nil
				}
			}
		}
		return nil, uuid.UUID{}, err
	}
	return target, inv.ID, nil
}

// resolveTargetPeriod implements ensureAdjustmentTarget's period resolution:
// reuse the earliest already-open period after p if one exists; otherwise
// ensure the current calendar month (in the teacher's timezone) — or, when
// "now" is not itself after p, the month immediately following p — rolling
// forward one further month if that candidate turns out already closed.
func (s *Service) resolveTargetPeriod(ctx context.Context, periodScope authctx.Scope, p *Period) (*Period, error) {
	target, err := s.repo.NextOpenPeriod(ctx, periodScope, p.PeriodEnd)
	if err != nil {
		return nil, err
	}
	if target != nil {
		return target, nil
	}

	loc, err := s.teacherLocation(ctx, periodScope.TeacherID)
	if err != nil {
		return nil, err
	}
	today := dateOnly(s.now().In(loc))
	year, month := today.Year(), int(today.Month())
	candidateStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	if !candidateStart.After(p.PeriodEnd) {
		year, month = nextCalendarMonth(int(p.Year), int(p.Month))
	}

	// A rollover period ensured here belongs to the same teacher/center the
	// closed period p belonged to — periodScope carries exactly that,
	// so EnsurePeriod's create-assigns-self stamp lands on the right tenant
	// even when this call originated from an owner's or a different
	// teacher's attendance edit.
	ensured, err := s.EnsurePeriod(ctx, periodScope, year, month)
	if err != nil {
		return nil, err
	}
	if ensured.Status == PeriodClosed {
		year, month = nextCalendarMonth(year, month)
		ensured, err = s.EnsurePeriod(ctx, periodScope, year, month)
		if err != nil {
			return nil, err
		}
	}
	return ensured, nil
}
