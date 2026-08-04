package statements

import (
	"fmt"
	"strings"
)

// ChildSummary is one child's line in a parent-facing message: the class
// count and amount for the period, exactly as ContactFigures.Children
// carries them.
type ChildSummary struct {
	StudentName   string
	BillableCount int
	AbsentCount   int
	Amount        int64
}

// MessageInput is everything Build needs to render one family's
// parent-facing statement message. It carries no db handle, clock, or
// config — every field is a value the caller (notifications.Service)
// already resolved, so Build stays a pure function of its input.
type MessageInput struct {
	ContactName     string
	PeriodLabel     string
	Children        []ChildSummary
	OpeningBalance  int64
	AdjustmentTotal int64
	TotalDue        int64
	Outstanding     int64
	URL             string
}

// Build renders the Vietnamese-language parent statement message for in.
// The URL is always the last line and is never dropped, even when the
// message must collapse per-child detail to fit maxLen. collapsed reports
// whether that collapse happened.
func Build(in MessageInput, maxLen int) (text string, collapsed bool) {
	full := render(in, false)
	if maxLen <= 0 || len(full) <= maxLen {
		return full, false
	}
	return render(in, true), true
}

// render assembles the message body. When collapse is true, per-child lines
// are replaced by a single summary line; every other line (greeting,
// opening balance, adjustment, total, url) is unchanged.
func render(in MessageInput, collapse bool) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Chào anh/chị %s,\n", in.ContactName)
	fmt.Fprintf(&b, "Học phí tháng %s:\n", in.PeriodLabel)

	if collapse {
		childCount := len(in.Children)
		var sessionSum int
		for _, c := range in.Children {
			sessionSum += c.BillableCount
		}
		fmt.Fprintf(&b, "%d bạn, tổng %d buổi học\n", childCount, sessionSum)
	} else {
		for _, c := range in.Children {
			b.WriteString(childLine(c))
			b.WriteString("\n")
		}
	}

	if in.OpeningBalance != 0 {
		fmt.Fprintf(&b, "Nợ cũ: %s\n", formatMoney(in.OpeningBalance))
	}

	if in.AdjustmentTotal != 0 {
		fmt.Fprintf(&b, "Điều chỉnh: %s\n", formatSignedMoney(in.AdjustmentTotal))
	}

	fmt.Fprintf(&b, "Tổng cộng: %s\n", formatMoney(in.TotalDue))
	fmt.Fprintf(&b, "Chi tiết từng buổi: %s", in.URL)

	return b.String()
}

// childLine renders one child's summary line. AbsentCount only appears when
// there were absences — a child with a perfect attendance record gets a
// shorter, cleaner line.
func childLine(c ChildSummary) string {
	if c.AbsentCount > 0 {
		return fmt.Sprintf("- %s: %d buổi học, %d buổi vắng — %s", c.StudentName, c.BillableCount, c.AbsentCount, formatMoney(c.Amount))
	}
	return fmt.Sprintf("- %s: %d buổi học — %s", c.StudentName, c.BillableCount, formatMoney(c.Amount))
}

// formatMoney renders a non-negative VND amount grouped by thousands with a
// trailing đ suffix, e.g. 1234567 -> "1.234.567 đ". Negative input is
// rendered with its sign preserved (formatSignedMoney is the usual caller
// for signed amounts, but formatMoney never panics on a negative value).
func formatMoney(amount int64) string {
	neg := amount < 0
	if neg {
		amount = -amount
	}
	digits := fmt.Sprintf("%d", amount)

	var grouped strings.Builder
	n := len(digits)
	for i, d := range digits {
		if i > 0 && (n-i)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(d)
	}

	if neg {
		return "-" + grouped.String() + " đ"
	}
	return grouped.String() + " đ"
}

// formatSignedMoney renders amount the same way as formatMoney but always
// with an explicit leading sign (+/-) — used for the adjustment line, where
// the direction of the adjustment matters as much as its size.
func formatSignedMoney(amount int64) string {
	if amount >= 0 {
		return "+" + formatMoney(amount)
	}
	return formatMoney(amount)
}
