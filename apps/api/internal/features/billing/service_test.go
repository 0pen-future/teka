package billing

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// --- fake Repository ---

type fakeRepository struct {
	periods   map[uuid.UUID]Period
	timezones map[uuid.UUID]string
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{periods: map[uuid.UUID]Period{}, timezones: map[uuid.UUID]string{}}
}

func (f *fakeRepository) setTimezone(teacherID uuid.UUID, tz string) {
	f.timezones[teacherID] = tz
}

func (f *fakeRepository) CreatePeriod(_ context.Context, p *Period) error {
	for _, existing := range f.periods {
		if existing.TeacherID == p.TeacherID && existing.Year == p.Year && existing.Month == p.Month {
			return ErrDuplicatePeriod
		}
	}
	f.periods[p.ID] = *p
	return nil
}

func (f *fakeRepository) GetPeriod(_ context.Context, teacherID, periodID uuid.UUID) (*Period, error) {
	p, ok := f.periods[periodID]
	if !ok || p.TeacherID != teacherID {
		return nil, ErrPeriodNotFound
	}
	cp := p
	return &cp, nil
}

func (f *fakeRepository) GetPeriodByYearMonth(_ context.Context, teacherID uuid.UUID, year, month int16) (*Period, error) {
	for _, p := range f.periods {
		if p.TeacherID == teacherID && p.Year == year && p.Month == month {
			cp := p
			return &cp, nil
		}
	}
	return nil, ErrPeriodNotFound
}

func (f *fakeRepository) ListPeriods(_ context.Context, teacherID uuid.UUID, _ pagination.Params) ([]Period, int64, error) {
	var out []Period
	for _, p := range f.periods {
		if p.TeacherID == teacherID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PeriodStart.After(out[j].PeriodStart) })
	return out, int64(len(out)), nil
}

func (f *fakeRepository) PreviousClosedPeriod(_ context.Context, teacherID uuid.UUID, before time.Time) (*Period, error) {
	var best *Period
	for _, p := range f.periods {
		row := p
		if row.TeacherID != teacherID || row.Status != PeriodClosed || !row.PeriodEnd.Before(before) {
			continue
		}
		if best == nil || row.PeriodEnd.After(best.PeriodEnd) {
			best = &row
		}
	}
	return best, nil
}

func (f *fakeRepository) TallyAttendance(_ context.Context, _, _ uuid.UUID) ([]AttendanceTally, error) {
	return nil, nil
}

func (f *fakeRepository) OpeningBalances(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) (map[uuid.UUID]int64, error) {
	return map[uuid.UUID]int64{}, nil
}

func (f *fakeRepository) TeacherTimezone(_ context.Context, teacherID uuid.UUID) (string, error) {
	tz, ok := f.timezones[teacherID]
	if !ok {
		return "", ErrTeacherNotFound
	}
	return tz, nil
}

// The stubs below satisfy the phase 2 additions to Repository for tests in
// this file that only exercise EnsurePeriod/GetPeriod/ListPeriods; preview_test.go
// defines a richer fake (previewFakeRepository, embedding this one) for the
// Preview/Draft behaviour these stubs deliberately do not model.

func (f *fakeRepository) AdjustmentTotals(_ context.Context, _, _ uuid.UUID) (map[uuid.UUID]int64, error) {
	return map[uuid.UUID]int64{}, nil
}

func (f *fakeRepository) CarriedDebtStudents(_ context.Context, _, _ uuid.UUID) ([]CarriedDebtStudent, error) {
	return nil, nil
}

func (f *fakeRepository) UpsertInvoice(_ context.Context, _ *Invoice) error { return nil }

func (f *fakeRepository) UpsertInvoiceLine(_ context.Context, _ *InvoiceLine) error { return nil }

func (f *fakeRepository) ZeroUnmatchedLines(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) error {
	return nil
}

func (f *fakeRepository) ListInvoices(_ context.Context, _, _ uuid.UUID) ([]Invoice, error) {
	return nil, nil
}

func (f *fakeRepository) GetInvoiceWithLines(_ context.Context, _, _ uuid.UUID) (*Invoice, []InvoiceLine, error) {
	return nil, nil, ErrInvoiceNotFound
}

// The stubs below satisfy the phase 3 additions to Repository for tests in
// this file that only exercise EnsurePeriod/GetPeriod/ListPeriods; close_test.go
// defines a richer fake (closeFakeRepository, embedding previewFakeRepository)
// for the Close/VoidInvoice behaviour these stubs deliberately do not model.

func (f *fakeRepository) LockPeriod(ctx context.Context, teacherID, periodID uuid.UUID) (*Period, error) {
	return f.GetPeriod(ctx, teacherID, periodID)
}

func (f *fakeRepository) IssueDraftInvoices(_ context.Context, _, _ uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *fakeRepository) VoidInvoices(_ context.Context, _, _ uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *fakeRepository) ClosePeriod(_ context.Context, teacherID, periodID uuid.UUID, closedAt time.Time) error {
	p, ok := f.periods[periodID]
	if !ok || p.TeacherID != teacherID || p.Status != PeriodOpen {
		return errPeriodStatusChanged
	}
	p.Status = PeriodClosed
	p.ClosedAt = &closedAt
	f.periods[periodID] = p
	return nil
}

func (f *fakeRepository) GetInvoice(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
	return nil, ErrInvoiceNotFound
}

func (f *fakeRepository) LockInvoice(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
	return nil, ErrInvoiceNotFound
}

func (f *fakeRepository) VoidInvoice(_ context.Context, _, _ uuid.UUID, _ string, _ time.Time) error {
	return ErrInvoiceNotFound
}

// The stubs below satisfy the phase 4 additions to Repository for tests in
// this file and preview_test.go that never exercise AddAdjustment or
// ReconcileSession; those flows are covered by billing's integration tests
// against a real database instead of a richer fake.

func (f *fakeRepository) CreateAdjustment(_ context.Context, _ *InvoiceAdjustment) error { return nil }

func (f *fakeRepository) ListAdjustments(_ context.Context, _, _ uuid.UUID) ([]InvoiceAdjustment, error) {
	return nil, nil
}

func (f *fakeRepository) AdjustmentsBySourcePeriod(_ context.Context, _, _, _ uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *fakeRepository) RecalcInvoiceTotals(_ context.Context, _, _ uuid.UUID) error {
	return ErrInvoiceNotFound
}

func (f *fakeRepository) PeriodContainingDate(_ context.Context, _ uuid.UUID, _ time.Time) (*Period, error) {
	return nil, nil
}

func (f *fakeRepository) NextOpenPeriod(_ context.Context, _ uuid.UUID, _ time.Time) (*Period, error) {
	return nil, nil
}

func (f *fakeRepository) LiveBillableCounts(_ context.Context, _ uuid.UUID, _ []uuid.UUID, _ *Period) (map[uuid.UUID]int, error) {
	return map[uuid.UUID]int{}, nil
}

func (f *fakeRepository) SessionMeta(_ context.Context, _, _ uuid.UUID) (uuid.UUID, string, time.Time, error) {
	return uuid.UUID{}, "", time.Time{}, ErrSessionNotFound
}

func (f *fakeRepository) StudentSnapshot(_ context.Context, _, _ uuid.UUID) (uuid.UUID, string, string, error) {
	return uuid.UUID{}, "", "", ErrStudentNotFound
}

func (f *fakeRepository) SessionAttendance(_ context.Context, _, _ uuid.UUID) ([]attendance.Record, error) {
	return nil, nil
}

// --- noopTx ---

type noopTx struct{}

func (noopTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// --- fake PendingSource ---

// fakePendingSource lets tests control what billing's period close sees as
// unconfirmed sessions without depending on the real sessions package.
// respond defaults to an empty, non-blocking result when nil.
type fakePendingSource struct {
	respond func(from, to *time.Time, before time.Time) (*sessions.PendingResponse, error)
}

func (f *fakePendingSource) ListUnconfirmedInWindow(_ context.Context, _ authctx.Scope, from, to *time.Time, before time.Time, _ int) (*sessions.PendingResponse, error) {
	if f.respond == nil {
		return &sessions.PendingResponse{}, nil
	}
	return f.respond(from, to, before)
}

func newTestService() (*Service, *fakeRepository) {
	repo := newFakeRepository()
	return NewService(repo, noopTx{}, &fakePendingSource{}, nil), repo
}

func TestEnsurePeriodComputesFirstAndLastDayInTeacherTimezone(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	repo.setTimezone(teacherID, "Asia/Ho_Chi_Minh")

	// February 2026 is not a leap year (2026 % 4 != 0): 28 days.
	period, err := svc.EnsurePeriod(ctx, teacherID, 2026, 2)
	if err != nil {
		t.Fatalf("ensure period: %v", err)
	}
	if period.Status != PeriodOpen {
		t.Fatalf("new period must open, got %s", period.Status)
	}
	if got := period.PeriodStart.Format(dateLayout); got != "2026-02-01" {
		t.Fatalf("period_start = %s, want 2026-02-01", got)
	}
	if got := period.PeriodEnd.Format(dateLayout); got != "2026-02-28" {
		t.Fatalf("period_end = %s, want 2026-02-28", got)
	}
}

func TestEnsurePeriodIsIdempotent(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	repo.setTimezone(teacherID, "Asia/Ho_Chi_Minh")

	first, err := svc.EnsurePeriod(ctx, teacherID, 2026, 3)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	second, err := svc.EnsurePeriod(ctx, teacherID, 2026, 3)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("idempotent EnsurePeriod must return the same id, got %s and %s", first.ID, second.ID)
	}
	if len(repo.periods) != 1 {
		t.Fatalf("must not create a second row, got %d periods", len(repo.periods))
	}
}

func TestEnsurePeriodFallsBackToUTCOnInvalidTimezone(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	repo.setTimezone(teacherID, "not-a-real-zone")

	period, err := svc.EnsurePeriod(ctx, teacherID, 2026, 1)
	if err != nil {
		t.Fatalf("an invalid stored timezone must not fail the request, got: %v", err)
	}
	if got := period.PeriodStart.Format(dateLayout); got != "2026-01-01" {
		t.Fatalf("period_start = %s, want 2026-01-01", got)
	}
}

func TestEnsurePeriodMissingTeacherIsNotFound(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	_, err := svc.EnsurePeriod(ctx, id.New(), 2026, 1)
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("missing teacher must be 404, got %v", err)
	}
}

func TestGetPeriodNotFound(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	_, err := svc.GetPeriod(ctx, id.New(), id.New())
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("missing period must be 404, got %v", err)
	}
}

func TestGetPeriodCrossTenantIsNotFound(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	owner, stranger := id.New(), id.New()
	repo.setTimezone(owner, "Asia/Ho_Chi_Minh")

	period, err := svc.EnsurePeriod(ctx, owner, 2026, 1)
	if err != nil {
		t.Fatalf("ensure period: %v", err)
	}

	_, err = svc.GetPeriod(ctx, stranger, period.ID)
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be 404, got %v", err)
	}
}

func TestListPeriodsScopedToTeacher(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherA, teacherB := id.New(), id.New()
	repo.setTimezone(teacherA, "Asia/Ho_Chi_Minh")
	repo.setTimezone(teacherB, "Asia/Ho_Chi_Minh")

	if _, err := svc.EnsurePeriod(ctx, teacherA, 2026, 1); err != nil {
		t.Fatalf("ensure period A: %v", err)
	}
	if _, err := svc.EnsurePeriod(ctx, teacherB, 2026, 1); err != nil {
		t.Fatalf("ensure period B: %v", err)
	}

	rows, total, err := svc.ListPeriods(ctx, teacherA, pagination.Params{})
	if err != nil {
		t.Fatalf("list periods: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("want exactly teacher A's own period, got total=%d rows=%d", total, len(rows))
	}
	if rows[0].TeacherID != teacherA {
		t.Fatalf("list must not leak another teacher's period, got %+v", rows[0])
	}
}
