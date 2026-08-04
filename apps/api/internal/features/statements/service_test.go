package statements

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// --- noopTx ---

// noopTx runs fn directly — the fakeRepository below has no real transaction
// boundary to join, so Generate's WithinTx wrapper is a pass-through in unit
// tests.
type noopTx struct{}

func (noopTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// --- fakeRepository ---

type statementKey struct {
	contactID uuid.UUID
	periodID  uuid.UUID
}

// fakeRepository is an in-memory Repository enforcing the same upsert
// invariant the SQL layer's partial unique index and guarded ON CONFLICT do:
// one row per (contact_id, period_id), a revoked row is never resurrected,
// and a refresh never touches token_hash.
type fakeRepository struct {
	periods map[uuid.UUID]string
	targets map[uuid.UUID][]TargetContact
	totals  map[uuid.UUID]map[uuid.UUID]int64

	byKey map[statementKey]*Statement
	byID  map[uuid.UUID]*Statement

	invoiceLines       map[statementKey][]InvoiceLineRow
	periodInvoiceLines map[uuid.UUID][]InvoiceLineRow
	liveSessions       map[statementKey][]LiveSessionRow
	adjustments        map[statementKey][]AdjustmentRow
	viewTouches        int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		periods: map[uuid.UUID]string{},
		targets: map[uuid.UUID][]TargetContact{},
		totals:  map[uuid.UUID]map[uuid.UUID]int64{},
		byKey:   map[statementKey]*Statement{},
		byID:    map[uuid.UUID]*Statement{},

		invoiceLines:       map[statementKey][]InvoiceLineRow{},
		periodInvoiceLines: map[uuid.UUID][]InvoiceLineRow{},
		liveSessions:       map[statementKey][]LiveSessionRow{},
		adjustments:        map[statementKey][]AdjustmentRow{},
	}
}

func (f *fakeRepository) GetPeriodStatus(_ context.Context, _, periodID uuid.UUID) (string, error) {
	status, ok := f.periods[periodID]
	if !ok {
		return "", ErrPeriodNotFound
	}
	return status, nil
}

func (f *fakeRepository) TargetContacts(_ context.Context, _, periodID uuid.UUID) ([]TargetContact, error) {
	return f.targets[periodID], nil
}

func (f *fakeRepository) ContactTotals(_ context.Context, _, periodID uuid.UUID) (map[uuid.UUID]int64, error) {
	return f.totals[periodID], nil
}

func (f *fakeRepository) UpsertStatement(_ context.Context, teacherID uuid.UUID, stmt *Statement) (created, skippedRevoked bool, err error) {
	stmt.TeacherID = teacherID
	key := statementKey{contactID: stmt.ContactID, periodID: stmt.PeriodID}

	existing, ok := f.byKey[key]
	if !ok {
		row := *stmt
		f.byKey[key] = &row
		f.byID[row.ID] = &row
		return true, false, nil
	}
	if existing.RevokedAt != nil {
		return false, true, nil
	}
	// Refresh only total_due/updated_at; the persisted id/token_hash win over
	// the freshly-built candidate, exactly like the SQL upsert's RETURNING.
	existing.TotalDue = stmt.TotalDue
	existing.UpdatedAt = time.Now()
	*stmt = *existing
	return false, false, nil
}

func (f *fakeRepository) ListByPeriod(_ context.Context, teacherID, periodID uuid.UUID, _ pagination.Params) ([]Row, int64, error) {
	var out []Row
	for _, s := range f.byID {
		if s.TeacherID == teacherID && s.PeriodID == periodID {
			out = append(out, Row{Statement: *s})
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeRepository) GetByID(_ context.Context, teacherID, statementID uuid.UUID) (*Row, error) {
	s, ok := f.byID[statementID]
	if !ok || s.TeacherID != teacherID {
		return nil, ErrNotFound
	}
	return &Row{Statement: *s}, nil
}

func (f *fakeRepository) GetByTokenHash(_ context.Context, tokenHash []byte) (*Statement, error) {
	for _, s := range f.byID {
		if string(s.TokenHash) == string(tokenHash) {
			cp := *s
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeRepository) Revoke(_ context.Context, teacherID, statementID uuid.UUID) error {
	s, ok := f.byID[statementID]
	if !ok || s.TeacherID != teacherID {
		return ErrNotFound
	}
	if s.RevokedAt == nil {
		now := time.Now()
		s.RevokedAt = &now
	}
	return nil
}

func (f *fakeRepository) InvoicesWithLines(_ context.Context, _, contactID, periodID uuid.UUID) ([]InvoiceLineRow, error) {
	return f.invoiceLines[statementKey{contactID: contactID, periodID: periodID}], nil
}

func (f *fakeRepository) PeriodInvoiceLines(_ context.Context, _, periodID uuid.UUID) ([]InvoiceLineRow, error) {
	return f.periodInvoiceLines[periodID], nil
}

func (f *fakeRepository) LiveSessions(_ context.Context, _, contactID, periodID uuid.UUID) ([]LiveSessionRow, error) {
	return f.liveSessions[statementKey{contactID: contactID, periodID: periodID}], nil
}

func (f *fakeRepository) Adjustments(_ context.Context, _, contactID, periodID uuid.UUID) ([]AdjustmentRow, error) {
	return f.adjustments[statementKey{contactID: contactID, periodID: periodID}], nil
}

func (f *fakeRepository) TouchView(_ context.Context, teacherID, statementID uuid.UUID) error {
	f.viewTouches++
	s, ok := f.byID[statementID]
	if !ok || s.TeacherID != teacherID {
		return nil
	}
	now := time.Now()
	if s.FirstViewedAt == nil {
		s.FirstViewedAt = &now
	}
	s.LastViewedAt = &now
	s.ViewCount++
	return nil
}

// --- tests ---

func testConfig() config.StatementsConfig {
	return config.StatementsConfig{
		TokenKey:      []byte("0123456789abcdef0123456789abcdef"),
		PublicBaseURL: "https://parent.example.com",
	}
}

func TestGenerateOpenPeriodIsConflict(t *testing.T) {
	repo := newFakeRepository()
	teacherID, periodID := id.New(), id.New()
	repo.periods[periodID] = "open"
	svc := NewService(repo, noopTx{}, testConfig(), testBankConfig(), NewQRBuilder())

	_, err := svc.Generate(context.Background(), teacherID, periodID)
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("want CONFLICT for an open period, got %v", err)
	}
}

func TestGenerateUnknownPeriodIsNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, noopTx{}, testConfig(), testBankConfig(), NewQRBuilder())

	_, err := svc.Generate(context.Background(), id.New(), id.New())
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("want NOT_FOUND for an unknown period, got %v", err)
	}
}

func TestGenerateCreatesOneStatementPerTargetContact(t *testing.T) {
	repo := newFakeRepository()
	teacherID, periodID := id.New(), id.New()
	contactA, contactB := id.New(), id.New()
	repo.periods[periodID] = periodStatusClosed
	repo.targets[periodID] = []TargetContact{
		{ContactID: contactA, FullName: "Chị Hoa", Phone: "+84912345678"},
		{ContactID: contactB, FullName: "Anh Tuấn", Phone: "+84912345679"},
	}
	repo.totals[periodID] = map[uuid.UUID]int64{contactA: 500_000, contactB: 750_000}
	svc := NewService(repo, noopTx{}, testConfig(), testBankConfig(), NewQRBuilder())

	result, err := svc.Generate(context.Background(), teacherID, periodID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if result.Created != 2 || result.Refreshed != 0 || result.SkippedRevoked != 0 {
		t.Fatalf("want 2 created, 0 refreshed, 0 skipped, got %+v", result)
	}
	if len(result.Statements) != 2 {
		t.Fatalf("want 2 statements returned, got %d", len(result.Statements))
	}
	for _, row := range result.Statements {
		if row.TeacherID != teacherID {
			t.Fatalf("statement must carry the calling teacher, got %s", row.TeacherID)
		}
		want := repo.totals[periodID][row.ContactID]
		if row.TotalDue != want {
			t.Fatalf("contact %s: total_due = %d, want %d", row.ContactID, row.TotalDue, want)
		}
	}
}

func TestGenerateTwiceRefreshesTotalDueKeepsToken(t *testing.T) {
	repo := newFakeRepository()
	teacherID, periodID := id.New(), id.New()
	contactID := id.New()
	repo.periods[periodID] = periodStatusClosed
	repo.targets[periodID] = []TargetContact{{ContactID: contactID, FullName: "Chị Hoa", Phone: "+84912345678"}}
	repo.totals[periodID] = map[uuid.UUID]int64{contactID: 500_000}
	svc := NewService(repo, noopTx{}, testConfig(), testBankConfig(), NewQRBuilder())
	ctx := context.Background()

	first, err := svc.Generate(ctx, teacherID, periodID)
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	firstStatement := first.Statements[0]

	repo.totals[periodID] = map[uuid.UUID]int64{contactID: 900_000}
	second, err := svc.Generate(ctx, teacherID, periodID)
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if second.Created != 0 || second.Refreshed != 1 {
		t.Fatalf("want 0 created, 1 refreshed on the second run, got %+v", second)
	}
	secondStatement := second.Statements[0]

	if secondStatement.ID != firstStatement.ID {
		t.Fatalf("refresh must keep the same statement id, got %s want %s", secondStatement.ID, firstStatement.ID)
	}
	if string(secondStatement.TokenHash) != string(firstStatement.TokenHash) {
		t.Fatal("refresh must never rotate token_hash — a parent's already-sent link would break")
	}
	if secondStatement.TotalDue != 900_000 {
		t.Fatalf("total_due must be refreshed, got %d", secondStatement.TotalDue)
	}
}

func TestGenerateSkipsRevokedStatementWithoutResurrectingIt(t *testing.T) {
	repo := newFakeRepository()
	teacherID, periodID := id.New(), id.New()
	contactID := id.New()
	repo.periods[periodID] = periodStatusClosed
	repo.targets[periodID] = []TargetContact{{ContactID: contactID, FullName: "Chị Hoa", Phone: "+84912345678"}}
	repo.totals[periodID] = map[uuid.UUID]int64{contactID: 500_000}
	svc := NewService(repo, noopTx{}, testConfig(), testBankConfig(), NewQRBuilder())
	ctx := context.Background()

	first, err := svc.Generate(ctx, teacherID, periodID)
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	if err := svc.Revoke(ctx, teacherID, first.Statements[0].ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	repo.totals[periodID] = map[uuid.UUID]int64{contactID: 999_000}
	second, err := svc.Generate(ctx, teacherID, periodID)
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if second.Created != 0 || second.Refreshed != 0 || second.SkippedRevoked != 1 {
		t.Fatalf("want the revoked statement skipped not resurrected, got %+v", second)
	}
	if len(second.Statements) != 0 {
		t.Fatalf("a skipped-revoked statement must not appear in the returned list, got %d", len(second.Statements))
	}

	stored, err := repo.GetByID(ctx, teacherID, first.Statements[0].ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.TotalDue != 500_000 {
		t.Fatalf("a revoked statement's total_due must stay frozen, got %d", stored.TotalDue)
	}
}

func TestListTranslatesUnknownPeriodAsNotFound(t *testing.T) {
	svc := NewService(newFakeRepository(), noopTx{}, testConfig(), testBankConfig(), NewQRBuilder())

	_, _, err := svc.List(context.Background(), id.New(), id.New(), pagination.Params{})
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("want NOT_FOUND, got %v", err)
	}
}

func TestRevokeIsIdempotent(t *testing.T) {
	repo := newFakeRepository()
	teacherID, periodID, contactID := id.New(), id.New(), id.New()
	repo.periods[periodID] = periodStatusClosed
	repo.targets[periodID] = []TargetContact{{ContactID: contactID, FullName: "Chị Hoa", Phone: "+84912345678"}}
	repo.totals[periodID] = map[uuid.UUID]int64{contactID: 500_000}
	svc := NewService(repo, noopTx{}, testConfig(), testBankConfig(), NewQRBuilder())
	ctx := context.Background()

	result, err := svc.Generate(ctx, teacherID, periodID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	statementID := result.Statements[0].ID

	if err := svc.Revoke(ctx, teacherID, statementID); err != nil {
		t.Fatalf("first revoke: %v", err)
	}
	if err := svc.Revoke(ctx, teacherID, statementID); err != nil {
		t.Fatalf("second revoke must be a no-op success, got %v", err)
	}
}

func TestRevokeOtherTeachersStatementIsNotFound(t *testing.T) {
	repo := newFakeRepository()
	teacherID, periodID, contactID := id.New(), id.New(), id.New()
	repo.periods[periodID] = periodStatusClosed
	repo.targets[periodID] = []TargetContact{{ContactID: contactID, FullName: "Chị Hoa", Phone: "+84912345678"}}
	repo.totals[periodID] = map[uuid.UUID]int64{contactID: 500_000}
	svc := NewService(repo, noopTx{}, testConfig(), testBankConfig(), NewQRBuilder())
	ctx := context.Background()

	result, err := svc.Generate(ctx, teacherID, periodID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	err = svc.Revoke(ctx, id.New(), result.Statements[0].ID)
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant revoke must be NOT_FOUND, got %v", err)
	}
}

func TestToResponseRecomputesURLFromToken(t *testing.T) {
	cfg := testConfig()
	svc := NewService(newFakeRepository(), noopTx{}, cfg, testBankConfig(), NewQRBuilder())
	row := Row{Statement: Statement{ID: id.New(), TotalDue: 500_000}}

	resp := svc.ToResponse(row)

	want := cfg.PublicBaseURL + "/s/" + deriveToken(cfg.TokenKey, row.ID)
	if resp.URL != want {
		t.Fatalf("url = %q, want %q", resp.URL, want)
	}
}
