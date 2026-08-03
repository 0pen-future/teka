package billing

import (
	"context"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/id"
)

// PeriodCompute is the R3 formula applied to one billing period — the shared
// calculation Preview serialises as-is and Draft persists (Architecture,
// phase 2). Computing it issues no writes, so Preview and Draft's read phase
// is identical; only Draft calls DraftPeriod afterward.
type PeriodCompute struct {
	Invoices []ComputedInvoice
	// TalliesByEnrollment supplies class_id and present_count, which
	// ComputedLine does not carry and the persisted invoice_lines table does
	// not store either — the wire response is built from this map, never
	// from a later read of the bare invoice_lines rows.
	TalliesByEnrollment map[uuid.UUID]AttendanceTally
	// ExistingByStudent is this period's already-stored invoices (from any
	// earlier draft or issue), keyed by student_id.
	ExistingByStudent map[uuid.UUID]Invoice
}

// ComputePeriod runs the R3 formula for one billing period: attendance tally
// + carried debt + adjustments in, one ComputedInvoice per student out. It is
// read-only — no repository method it calls writes anything — so Preview
// calls it directly and Draft calls it before opening its transaction.
func ComputePeriod(ctx context.Context, repo Repository, teacherID, periodID uuid.UUID) (*PeriodCompute, error) {
	period, err := repo.GetPeriod(ctx, teacherID, periodID)
	if err != nil {
		return nil, err
	}

	tallies, err := repo.TallyAttendance(ctx, teacherID, periodID)
	if err != nil {
		return nil, err
	}
	talliesByEnrollment := make(map[uuid.UUID]AttendanceTally, len(tallies))
	for _, t := range tallies {
		talliesByEnrollment[t.EnrollmentID] = t
	}

	prevPeriod, err := repo.PreviousClosedPeriod(ctx, teacherID, period.PeriodStart)
	if err != nil {
		return nil, err
	}

	var carriedDebt []CarriedDebtStudent
	openingBalances := map[uuid.UUID]int64{}
	if prevPeriod != nil {
		carriedDebt, err = repo.CarriedDebtStudents(ctx, teacherID, prevPeriod.ID)
		if err != nil {
			return nil, err
		}

		// Union of tally students and carried-debt students: OpeningBalances
		// must be asked about every student who might need an opening
		// balance, not only the ones with attendance this period — a
		// fully-departed debtor has no tally row at all.
		studentSet := make(map[uuid.UUID]struct{}, len(tallies)+len(carriedDebt))
		for _, t := range tallies {
			studentSet[t.StudentID] = struct{}{}
		}
		for _, c := range carriedDebt {
			studentSet[c.StudentID] = struct{}{}
		}
		studentIDs := make([]uuid.UUID, 0, len(studentSet))
		for sid := range studentSet {
			studentIDs = append(studentIDs, sid)
		}

		openingBalances, err = repo.OpeningBalances(ctx, teacherID, prevPeriod.ID, studentIDs)
		if err != nil {
			return nil, err
		}
	}

	existingInvoices, err := repo.ListInvoices(ctx, teacherID, periodID)
	if err != nil {
		return nil, err
	}
	existingByStudent := make(map[uuid.UUID]Invoice, len(existingInvoices))
	invoiceIDToStudent := make(map[uuid.UUID]uuid.UUID, len(existingInvoices))
	for _, inv := range existingInvoices {
		existingByStudent[inv.StudentID] = inv
		invoiceIDToStudent[inv.ID] = inv.StudentID
	}

	// Compute's adjustments parameter is keyed by student_id; the persisted
	// data is keyed by invoice_id. Remap through this period's own invoices
	// (empty on a first-ever preview/draft, so adjustmentsByStudent is empty
	// too — adjustment_total stays 0 until phase 4 attaches one).
	adjustmentsByInvoice, err := repo.AdjustmentTotals(ctx, teacherID, periodID)
	if err != nil {
		return nil, err
	}
	adjustmentsByStudent := make(map[uuid.UUID]int64, len(adjustmentsByInvoice))
	for invoiceID, total := range adjustmentsByInvoice {
		if studentID, ok := invoiceIDToStudent[invoiceID]; ok {
			adjustmentsByStudent[studentID] = total
		}
	}

	// A student who already has an invoice for this period (an earlier
	// draft) must be revisited even if this recompute finds no tally, no
	// carried debt, and no adjustment for them — e.g. every attendance
	// record behind their one line was corrected away. Compute only emits a
	// ComputedInvoice for a student key present in tallies, opening, or
	// adjustments, so force a zero entry through the adjustments map (a
	// student already carrying a real adjustment is untouched by the ok
	// check) so DraftPeriod's rewrite loop still reaches — and zeroes —
	// their stale invoice_lines instead of leaving them stuck at their last
	// computed amount.
	for studentID := range existingByStudent {
		if _, ok := adjustmentsByStudent[studentID]; !ok {
			adjustmentsByStudent[studentID] = 0
		}
	}

	invoices := Compute(tallies, openingBalances, adjustmentsByStudent)

	// Compute's "extra" path (a student with no tally row — carried debt
	// only, or an adjustment-only re-draft) leaves ContactID and both names
	// zero-valued because Compute has no metadata source for them. Fill them
	// in from the freshest source available: the carried-debt row's live
	// contact first, falling back to this period's own existing invoice
	// snapshot (an adjustment always implies an invoice already exists —
	// invoice_adjustments.invoice_id is a required FK).
	carriedDebtByStudent := make(map[uuid.UUID]CarriedDebtStudent, len(carriedDebt))
	for _, c := range carriedDebt {
		carriedDebtByStudent[c.StudentID] = c
	}
	for i := range invoices {
		inv := &invoices[i]
		if inv.ContactID != uuid.Nil {
			continue
		}
		if cd, ok := carriedDebtByStudent[inv.StudentID]; ok {
			inv.ContactID = cd.ContactID
			inv.StudentName = cd.StudentName
			inv.ContactName = cd.ContactName
			continue
		}
		if existing, ok := existingByStudent[inv.StudentID]; ok {
			inv.ContactID = existing.ContactID
			inv.StudentName = existing.StudentName
			inv.ContactName = existing.ContactName
		}
	}

	return &PeriodCompute{
		Invoices:            invoices,
		TalliesByEnrollment: talliesByEnrollment,
		ExistingByStudent:   existingByStudent,
	}, nil
}

// DraftPeriod persists compute.Invoices as invoices and invoice_lines. The
// caller must run it inside a transaction (database.TxManager.WithinTx) —
// every repository call it makes reads ctx for the transaction handle, so
// all four Architecture upsert steps land atomically.
//
// It re-reads each invoice's existing id, status, and true adjustment_total
// from the database at write time rather than trusting compute's outer
// snapshot (taken before the transaction opened), so a concurrent change is
// never silently overwritten. adjustment_total has no dependency on
// invoice_lines, so deriving it before writing lines — instead of the
// Architecture doc's literal read-after-write ordering — collapses steps 1
// and 4 into a single write per invoice without weakening any guarantee: a
// brand-new invoice cannot have adjustments yet (invoice_adjustments.invoice_id
// is a required FK), so its adjustment_total is correctly 0 either way.
//
// Returns the resolved invoice id for every drafted student, and
// ErrInvoiceNotDraft (unwrapped by the caller into a 409) the moment any
// targeted invoice turns out not to be a draft — nothing after that point
// has been written, and the transaction rolls back everything before it too.
func DraftPeriod(ctx context.Context, repo Repository, teacherID, periodID uuid.UUID, compute *PeriodCompute) (map[uuid.UUID]uuid.UUID, error) {
	existingInvoices, err := repo.ListInvoices(ctx, teacherID, periodID)
	if err != nil {
		return nil, err
	}
	existingByStudent := make(map[uuid.UUID]Invoice, len(existingInvoices))
	for _, inv := range existingInvoices {
		existingByStudent[inv.StudentID] = inv
	}

	for _, ci := range compute.Invoices {
		if existing, ok := existingByStudent[ci.StudentID]; ok && existing.Status != InvoiceDraft {
			return nil, ErrInvoiceNotDraft
		}
	}

	adjustmentsByInvoice, err := repo.AdjustmentTotals(ctx, teacherID, periodID)
	if err != nil {
		return nil, err
	}

	invoiceIDByStudent := make(map[uuid.UUID]uuid.UUID, len(compute.Invoices))
	for _, ci := range compute.Invoices {
		invoiceID := id.New()
		if existing, ok := existingByStudent[ci.StudentID]; ok {
			invoiceID = existing.ID
		}
		// A candidate id for a brand-new invoice never appears in
		// adjustmentsByInvoice, so this correctly resolves to 0 for it.
		adjustmentTotal := adjustmentsByInvoice[invoiceID]
		totalDue := ci.OpeningBalance + ci.CurrentCharge + adjustmentTotal

		inv := &Invoice{
			ID:              invoiceID,
			TeacherID:       teacherID,
			PeriodID:        periodID,
			StudentID:       ci.StudentID,
			ContactID:       ci.ContactID,
			StudentName:     ci.StudentName,
			ContactName:     ci.ContactName,
			OpeningBalance:  ci.OpeningBalance,
			CurrentCharge:   ci.CurrentCharge,
			AdjustmentTotal: adjustmentTotal,
			TotalDue:        totalDue,
			Status:          InvoiceDraft,
		}
		if err := repo.UpsertInvoice(ctx, inv); err != nil {
			return nil, err
		}
		invoiceIDByStudent[ci.StudentID] = inv.ID

		keepEnrollmentIDs := make([]uuid.UUID, 0, len(ci.Lines))
		for _, line := range ci.Lines {
			keepEnrollmentIDs = append(keepEnrollmentIDs, line.EnrollmentID)
			invLine := &InvoiceLine{
				ID:            id.New(),
				TeacherID:     teacherID,
				InvoiceID:     inv.ID,
				EnrollmentID:  line.EnrollmentID,
				ClassName:     line.ClassName,
				BillableCount: line.BillableCount,
				AbsentCount:   line.AbsentCount,
				UnitPrice:     line.UnitPrice,
				Amount:        line.Amount,
			}
			if err := repo.UpsertInvoiceLine(ctx, invLine); err != nil {
				return nil, err
			}
		}
		if err := repo.ZeroUnmatchedLines(ctx, teacherID, inv.ID, keepEnrollmentIDs); err != nil {
			return nil, err
		}
	}

	return invoiceIDByStudent, nil
}
