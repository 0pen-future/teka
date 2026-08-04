package statements

import (
	"fmt"

	"github.com/google/uuid"
)

// buildPublicStatement assembles the parent-facing payload from the three
// reads RenderPublic has already fetched: invoiceRows (snapshot money,
// grouped by invoice/child/class), sessionRows (live attendance), and
// adjustmentRows (direct plus carried-forward corrections). It is a pure
// function — no I/O — so render_test.go can exercise every branch (multiple
// children, multiple classes, an absent session, a carried adjustment) over
// fixture rows without a database. The QR block is left nil here; only
// RenderPublic, which owns the signing key and bank configuration, fills it
// in via attachQR.
func buildPublicStatement(invoiceRows []InvoiceLineRow, sessionRows []LiveSessionRow, adjustmentRows []AdjustmentRow) PublicStatement {
	sessionsByEnrollment := make(map[uuid.UUID][]PublicSession, len(sessionRows))
	for _, r := range sessionRows {
		sessionsByEnrollment[r.EnrollmentID] = append(sessionsByEnrollment[r.EnrollmentID], PublicSession{
			Date:    r.SessionDate.Format(dateLayout),
			Status:  r.AttendanceStatus,
			Counted: r.Billable,
		})
	}

	directByStudent := make(map[uuid.UUID][]PublicAdjustment, len(adjustmentRows))
	carriedByStudent := make(map[uuid.UUID]*PublicCarriedAdjustment, len(adjustmentRows))
	for _, a := range adjustmentRows {
		if a.Carried {
			c := carriedByStudent[a.StudentID]
			if c == nil {
				c = &PublicCarriedAdjustment{}
				carriedByStudent[a.StudentID] = c
			}
			c.Amount += a.Amount
			if a.SessionDate != nil {
				c.SessionDates = append(c.SessionDates, a.SessionDate.Format(dateLayout))
			}
			continue
		}
		kind := adjustmentKindManual
		if a.SourceSessionID != nil {
			kind = adjustmentKindCorrection
		}
		directByStudent[a.StudentID] = append(directByStudent[a.StudentID], PublicAdjustment{Amount: a.Amount, Kind: kind})
	}

	var contactName, period string
	var totals PublicTotals
	var payments PublicPayments
	children := make([]PublicChild, 0)

	// invoiceRows is ordered by student_name, invoice id, line created_at
	// (repository.go's ORDER BY) — one linear pass groups each invoice's
	// lines without a second sort.
	var current *PublicChild
	var currentInvoiceID uuid.UUID
	for _, row := range invoiceRows {
		if contactName == "" {
			contactName = row.ContactName
			period = fmt.Sprintf("%02d/%d", row.PeriodMonth, row.PeriodYear)
		}
		if current == nil || row.InvoiceID != currentInvoiceID {
			if current != nil {
				children = append(children, *current)
			}
			currentInvoiceID = row.InvoiceID

			adjustments := directByStudent[row.StudentID]
			if adjustments == nil {
				adjustments = []PublicAdjustment{}
			}
			current = &PublicChild{
				StudentName:       row.StudentName,
				DisplayNote:       row.DisplayNote,
				OpeningBalance:    row.OpeningBalance,
				Classes:           []PublicClass{},
				Adjustments:       adjustments,
				CarriedAdjustment: carriedByStudent[row.StudentID],
				Subtotal:          row.TotalDue,
			}

			outstanding := row.TotalDue - row.PaidAmount
			totals.OpeningBalance += row.OpeningBalance
			totals.CurrentCharge += row.CurrentCharge
			totals.AdjustmentTotal += row.AdjustmentTotal
			totals.TotalDue += row.TotalDue
			totals.Paid += row.PaidAmount
			totals.Outstanding += outstanding
			payments.TotalPaid += row.PaidAmount
			payments.ByInvoice = append(payments.ByInvoice, PublicInvoicePayment{
				StudentName: row.StudentName,
				TotalDue:    row.TotalDue,
				Paid:        row.PaidAmount,
				Outstanding: outstanding,
			})
		}
		if row.LineID != nil {
			current.Classes = append(current.Classes, PublicClass{
				ClassName:     *row.ClassName,
				UnitPrice:     *row.UnitPrice,
				BillableCount: *row.BillableCount,
				AbsentCount:   *row.AbsentCount,
				Amount:        *row.LineAmount,
				Sessions:      sessionsByEnrollment[*row.EnrollmentID],
			})
		}
	}
	if current != nil {
		children = append(children, *current)
	}

	return PublicStatement{
		ContactName: contactName,
		Period:      period,
		Children:    children,
		Totals:      totals,
		Payments:    payments,
	}
}

// attachQR fills in payload.QR from the teacher's bank configuration and
// returns the raw VietQR payload string the qr.png route renders — empty
// when the bank config is absent, in which case payload.QR is left nil
// rather than a placeholder. Pure given its inputs, so render_test.go can
// exercise the QR-absent branch without a database.
func attachQR(payload *PublicStatement, qr QRBuilder, bank BankConfig, publicBaseURL, token string) string {
	note := fmt.Sprintf("HP %s %s", payload.ContactName, payload.Period)
	qrPayload, ok := qr.Payload(bank, payload.Totals.Outstanding, note)
	if !ok {
		return ""
	}
	payload.QR = &PublicQR{
		ImageURL: publicBaseURL + "/public/statements/" + token + "/qr.png",
		Amount:   payload.Totals.Outstanding,
		Note:     note,
	}
	return qrPayload
}
