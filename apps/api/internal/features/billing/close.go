package billing

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

// closeFeedLimit is the cap passed to PendingSource.ListUnconfirmedInWindow
// for both the blocking check and the future-warnings scan. Close must be
// able to name every unconfirmed session in the period (R4), not a
// truncated page — sessions.Service still clamps this to its own internal
// maxPendingLimit, so the blocking decision below reads resp.Total, never
// len(resp.Items), to stay correct even if the named list is truncated.
const closeFeedLimit = 1000

// PendingSource is the slice of the sessions feature billing's period close
// needs: the unconfirmed-sessions predicate with an explicit `before`
// cutoff. *sessions.Service satisfies this. Declared here — a
// consumer-defined interface, the same pattern billing uses for
// AttendanceSource — so billing depends on sessions' public service
// contract, never its repository or a session-scanning query of its own.
type PendingSource interface {
	ListUnconfirmedInWindow(ctx context.Context, sc authctx.Scope, from, to *time.Time, before time.Time, limit int) (*sessions.PendingResponse, error)
}

// ErrUnconfirmedSessions is Close's blocking error (R4): the period has at
// least one past session without confirmed attendance. It is returned
// unwrapped rather than as an *apperror.AppError so it can carry the
// offending list across the tx.WithinTx boundary — apperror.Fields is a flat
// map[string]string and cannot hold it. The handler maps this onto 409 with
// a details payload via response.ErrWithDetails.
type ErrUnconfirmedSessions struct {
	Sessions []UnconfirmedSession
}

func (e *ErrUnconfirmedSessions) Error() string {
	return "period has unconfirmed sessions"
}

// mapUnconfirmedSessions renders a pending feed response onto billing's wire
// shape, ordered by session_date then class_name (Implementation Steps).
func mapUnconfirmedSessions(resp *sessions.PendingResponse) []UnconfirmedSession {
	out := make([]UnconfirmedSession, 0, len(resp.Items))
	for _, item := range resp.Items {
		out = append(out, UnconfirmedSession{
			SessionID:   item.SessionID,
			ClassID:     item.ClassID,
			ClassName:   item.ClassName,
			SessionDate: item.SessionDate,
			Status:      item.Status,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SessionDate != out[j].SessionDate {
			return out[i].SessionDate < out[j].SessionDate
		}
		return out[i].ClassName < out[j].ClassName
	})
	return out
}

// blockingSessions returns every past session in the period without
// confirmed attendance — the R4 hard block. before=today (the same cutoff
// ListPending resolves for the dashboard) so this call is byte-identical to
// what the dashboard's own feed would return over the same from/to window;
// that agreement is what plan 03's contract promises and this package's
// integration tests assert.
func blockingSessions(ctx context.Context, pending PendingSource, teacherID uuid.UUID, period *Period, today time.Time) ([]UnconfirmedSession, error) {
	from := period.PeriodStart
	to := period.PeriodEnd
	if today.Before(to) {
		to = today
	}
	// Billing has not been re-keyed to center scope yet; this shim carries
	// only the teacher id, so sessions' scoped query still resolves tenancy
	// by teacher until billing gets its own sweep.
	resp, err := pending.ListUnconfirmedInWindow(ctx, authctx.Scope{TeacherID: teacherID}, &from, &to, today, closeFeedLimit)
	if err != nil {
		return nil, err
	}
	if resp.Total == 0 {
		return nil, nil
	}
	return mapUnconfirmedSessions(resp), nil
}

// futureUnconfirmedSessions returns every session dated after today but
// still inside the period without confirmed attendance — informational only
// (Close does not block on these). before=period_end+1 day so a session
// dated exactly on the period's last day is included (session_date < before
// == session_date <= period_end).
func futureUnconfirmedSessions(ctx context.Context, pending PendingSource, teacherID uuid.UUID, period *Period, today time.Time) ([]UnconfirmedSession, error) {
	from := today.AddDate(0, 0, 1)
	if from.After(period.PeriodEnd) {
		return nil, nil
	}
	to := period.PeriodEnd
	before := period.PeriodEnd.AddDate(0, 0, 1)
	// Billing has not been re-keyed to center scope yet; this shim carries
	// only the teacher id, so sessions' scoped query still resolves tenancy
	// by teacher until billing gets its own sweep.
	resp, err := pending.ListUnconfirmedInWindow(ctx, authctx.Scope{TeacherID: teacherID}, &from, &to, before, closeFeedLimit)
	if err != nil {
		return nil, err
	}
	return mapUnconfirmedSessions(resp), nil
}

// Close runs the seven-step chốt sổ pipeline inside one transaction: lock the
// period, refuse if any past session lacks confirmed attendance, compute and
// draft every invoice, void the empty ones, issue the rest, then mark the
// period closed. A failure at any step rolls back everything before it —
// the period is only ever seen as open or fully closed, never partially so.
//
// today is resolved once, from the teacher's timezone, before the
// transaction opens, and threaded through every step that needs "now" as a
// calendar day — nothing inside the pipeline calls time.Now() itself.
func (s *Service) Close(ctx context.Context, teacherID, periodID uuid.UUID) (*CloseResponse, error) {
	loc, err := s.teacherLocation(ctx, teacherID)
	if err != nil {
		return nil, err
	}
	today := dateOnly(s.now().In(loc))
	closedAt := s.now()

	var (
		period      *Period
		issuedCount int64
		voidedCount int64
		totalDue    int64
		warnings    []UnconfirmedSession
	)

	err = s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		locked, lockErr := s.repo.LockPeriod(txCtx, teacherID, periodID)
		if lockErr != nil {
			return lockErr
		}
		if locked.Status != PeriodOpen {
			return apperror.Conflict("period is not open")
		}
		period = locked

		blocked, blockErr := blockingSessions(txCtx, s.pending, teacherID, period, today)
		if blockErr != nil {
			return blockErr
		}
		if len(blocked) > 0 {
			return &ErrUnconfirmedSessions{Sessions: blocked}
		}

		compute, computeErr := ComputePeriod(txCtx, s.repo, teacherID, periodID)
		if computeErr != nil {
			return computeErr
		}
		if _, draftErr := DraftPeriod(txCtx, s.repo, teacherID, periodID, compute); draftErr != nil {
			return draftErr
		}

		voidedCount, err = s.repo.VoidInvoices(txCtx, teacherID, periodID)
		if err != nil {
			return err
		}
		issuedCount, err = s.repo.IssueDraftInvoices(txCtx, teacherID, periodID)
		if err != nil {
			return err
		}

		if closeErr := s.repo.ClosePeriod(txCtx, teacherID, periodID, closedAt); closeErr != nil {
			return closeErr
		}
		period.Status = PeriodClosed
		period.ClosedAt = &closedAt

		invoices, listErr := s.repo.ListInvoices(txCtx, teacherID, periodID)
		if listErr != nil {
			return listErr
		}
		for _, inv := range invoices {
			if inv.Status != InvoiceVoid {
				totalDue += inv.TotalDue
			}
		}

		var warnErr error
		warnings, warnErr = futureUnconfirmedSessions(txCtx, s.pending, teacherID, period, today)
		return warnErr
	})

	var blocked *ErrUnconfirmedSessions
	if errors.As(err, &blocked) {
		return nil, blocked
	}
	if errors.Is(err, ErrPeriodNotFound) {
		return nil, apperror.NotFound("billing period")
	}
	if errors.Is(err, ErrInvoiceNotDraft) {
		return nil, apperror.Conflict("an invoice for this period changed concurrently")
	}
	if err != nil {
		return nil, apperror.From(err)
	}

	return &CloseResponse{
		Period:      FromPeriodModel(period),
		IssuedCount: issuedCount,
		VoidedCount: voidedCount,
		TotalDue:    totalDue,
		Warnings:    CloseWarnings{FutureUnconfirmedSessions: warnings},
	}, nil
}

// VoidInvoice corrects an issued (or partially paid) invoice, the only path
// available once an invoice can no longer be re-drafted (schema note (i)).
// The period may already be closed — that is the normal case. Refuses
// (409) an invoice that still carries a payment: the payment must be
// reversed first (plan 05), or voiding would leave allocated money pointing
// at a void invoice.
func (s *Service) VoidInvoice(ctx context.Context, teacherID, invoiceID uuid.UUID, reason string) (*InvoiceResponse, error) {
	// Re-validate the reason for parity with AddAdjustment: invoices.void_reason
	// has no DB CHECK backstop, so a non-HTTP caller that skips Gin binding must
	// not be able to persist an empty or over-long void reason.
	if err := validateAdjustmentReason(reason); err != nil {
		return nil, err
	}

	inv, err := s.repo.GetInvoice(ctx, teacherID, invoiceID)
	if errors.Is(err, ErrInvoiceNotFound) {
		return nil, apperror.NotFound("invoice")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if inv.Status != InvoiceIssued && inv.Status != InvoicePartiallyPaid {
		return nil, apperror.Conflict("invoice must be issued or partially paid to be voided")
	}
	if inv.PaidAmount != 0 {
		return nil, apperror.Conflict("invoice has a recorded payment; reverse the payment before voiding")
	}

	at := s.now()
	err = s.repo.VoidInvoice(ctx, teacherID, invoiceID, reason, at)
	if errors.Is(err, ErrInvoiceNotFound) {
		// The two guards above passed, but the guarded UPDATE still affected
		// no row — the invoice's status or paid_amount changed concurrently
		// between the read and the write. Reported as a conflict, not a 404:
		// the invoice does exist, just no longer in the state this call saw.
		return nil, apperror.Conflict("invoice state changed concurrently; reload and retry")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}

	updated, err := s.repo.GetInvoice(ctx, teacherID, invoiceID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	resp := FromInvoiceModel(updated)
	return &resp, nil
}
