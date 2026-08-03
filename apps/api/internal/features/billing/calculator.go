package billing

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

// AttendanceTally is one enrollment's billable/absent/present counts for a
// billing period, already zipped with the pricing and naming data billing
// needs — the assembled input to Compute. Counts come from plan 03's
// attendance.Service.TallyByEnrollment; nothing in this package re-derives
// them.
type AttendanceTally struct {
	EnrollmentID   uuid.UUID
	StudentID      uuid.UUID
	ContactID      uuid.UUID
	StudentName    string
	ContactName    string
	ClassID        uuid.UUID
	ClassName      string
	ClassStartDate time.Time
	UnitPrice      int64
	BillableCount  int
	AbsentCount    int
	PresentCount   int
}

// ComputedLine is one enrollment's charge within a computed invoice.
type ComputedLine struct {
	EnrollmentID  uuid.UUID
	ClassName     string
	UnitPrice     int64
	BillableCount int
	AbsentCount   int
	// Amount = BillableCount * UnitPrice — mirrors the DB CHECK at
	// docs/schema_design.sql:334.
	Amount int64
}

// ComputedInvoice is one student's fee computation for a billing period: the
// R3 formula applied, but not yet persisted.
type ComputedInvoice struct {
	StudentID   uuid.UUID
	ContactID   uuid.UUID
	StudentName string
	ContactName string
	Lines       []ComputedLine
	// OpeningBalance is the carried-over outstanding from the student's most
	// recently closed period, supplied by the caller (Repository.OpeningBalances).
	OpeningBalance int64
	// CurrentCharge = SUM(Lines[].Amount).
	CurrentCharge int64
	// AdjustmentTotal is supplied by the caller; V1 has no adjustments yet at
	// compute time (they are applied post-close), so it is normally 0 here.
	AdjustmentTotal int64
	// TotalDue = OpeningBalance + CurrentCharge + AdjustmentTotal — mirrors
	// the DB CHECK at docs/schema_design.sql:308.
	TotalDue int64
}

// lineWithStartDate pairs a ComputedLine with the sort keys (ClassStartDate,
// ClassName) that decide its position in the output, discarded once sorting
// is done — ComputedLine itself carries no start date.
type lineWithStartDate struct {
	line      ComputedLine
	startDate time.Time
}

type invoiceBuilder struct {
	studentID   uuid.UUID
	contactID   uuid.UUID
	studentName string
	contactName string
	lines       []lineWithStartDate
}

// Compute groups tallies by student and applies the R3 formula: for each
// student, one ComputedLine per enrollment and one ComputedInvoice summing
// them plus the carried opening balance and any adjustment total. It is pure:
// no I/O, no clock reads, deterministic output for the same input.
//
// A student present only in opening or adjustments (no tallies at all — a
// zero-session period, or carried debt with no new charge) still gets a
// ComputedInvoice with empty Lines.
func Compute(tallies []AttendanceTally, opening map[uuid.UUID]int64, adjustments map[uuid.UUID]int64) []ComputedInvoice {
	builders := make(map[uuid.UUID]*invoiceBuilder)
	var order []uuid.UUID

	ensure := func(studentID, contactID uuid.UUID, studentName, contactName string) *invoiceBuilder {
		b, ok := builders[studentID]
		if !ok {
			b = &invoiceBuilder{studentID: studentID, contactID: contactID, studentName: studentName, contactName: contactName}
			builders[studentID] = b
			order = append(order, studentID)
		}
		return b
	}

	for _, t := range tallies {
		b := ensure(t.StudentID, t.ContactID, t.StudentName, t.ContactName)
		amount := int64(t.BillableCount) * t.UnitPrice
		b.lines = append(b.lines, lineWithStartDate{
			line: ComputedLine{
				EnrollmentID:  t.EnrollmentID,
				ClassName:     t.ClassName,
				UnitPrice:     t.UnitPrice,
				BillableCount: t.BillableCount,
				AbsentCount:   t.AbsentCount,
				Amount:        amount,
			},
			startDate: t.ClassStartDate,
		})
	}

	// Students who only carry an opening balance or an adjustment (no
	// tallies at all) still need an invoice. Iterate both maps in a stable,
	// sorted order so output is deterministic even though Go map iteration
	// is not.
	extra := make(map[uuid.UUID]bool)
	for id := range opening {
		if _, ok := builders[id]; !ok {
			extra[id] = true
		}
	}
	for id := range adjustments {
		if _, ok := builders[id]; !ok {
			extra[id] = true
		}
	}
	extraIDs := make([]uuid.UUID, 0, len(extra))
	for id := range extra {
		extraIDs = append(extraIDs, id)
	}
	sort.Slice(extraIDs, func(i, j int) bool { return extraIDs[i].String() < extraIDs[j].String() })
	for _, id := range extraIDs {
		ensure(id, uuid.UUID{}, "", "")
	}

	out := make([]ComputedInvoice, 0, len(order))
	for _, studentID := range order {
		b := builders[studentID]
		sort.SliceStable(b.lines, func(i, j int) bool {
			if !b.lines[i].startDate.Equal(b.lines[j].startDate) {
				return b.lines[i].startDate.Before(b.lines[j].startDate)
			}
			return b.lines[i].line.ClassName < b.lines[j].line.ClassName
		})

		lines := make([]ComputedLine, len(b.lines))
		var currentCharge int64
		for i, lw := range b.lines {
			lines[i] = lw.line
			currentCharge += lw.line.Amount
		}

		openingBalance := opening[studentID]
		adjustmentTotal := adjustments[studentID]
		out = append(out, ComputedInvoice{
			StudentID:       b.studentID,
			ContactID:       b.contactID,
			StudentName:     b.studentName,
			ContactName:     b.contactName,
			Lines:           lines,
			OpeningBalance:  openingBalance,
			CurrentCharge:   currentCharge,
			AdjustmentTotal: adjustmentTotal,
			TotalDue:        openingBalance + currentCharge + adjustmentTotal,
		})
	}
	return out
}
