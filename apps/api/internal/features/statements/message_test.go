package statements

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".golden"))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(data)
}

func assertURLLast(t *testing.T, text, url string) {
	t.Helper()
	lines := strings.Split(text, "\n")
	last := lines[len(lines)-1]
	if last != "Chi tiết từng buổi: "+url {
		t.Fatalf("expected url line last, got %q", last)
	}
}

func TestBuildOneChildNoAbsences(t *testing.T) {
	url := "https://parent.example.com/s/abc123"
	in := MessageInput{
		ContactName: "Chị Lan",
		PeriodLabel: "08/2025",
		Children: []ChildSummary{
			{StudentName: "An", BillableCount: 8, AbsentCount: 0, Amount: 800000},
		},
		TotalDue: 800000,
		URL:      url,
	}

	text, collapsed := Build(in, 1000)

	if collapsed {
		t.Fatal("expected no collapse")
	}
	if want := loadGolden(t, "one_child_no_absences"); text != want {
		t.Fatalf("text mismatch:\ngot:  %q\nwant: %q", text, want)
	}
	assertURLLast(t, text, url)
}

func TestBuildOneChildWithAbsences(t *testing.T) {
	url := "https://parent.example.com/s/abc123"
	in := MessageInput{
		ContactName: "Chị Lan",
		PeriodLabel: "08/2025",
		Children: []ChildSummary{
			{StudentName: "Bình", BillableCount: 6, AbsentCount: 2, Amount: 600000},
		},
		TotalDue: 600000,
		URL:      url,
	}

	text, collapsed := Build(in, 1000)

	if collapsed {
		t.Fatal("expected no collapse")
	}
	if want := loadGolden(t, "one_child_with_absences"); text != want {
		t.Fatalf("text mismatch:\ngot:  %q\nwant: %q", text, want)
	}
	assertURLLast(t, text, url)
}

func TestBuildTwoChildren(t *testing.T) {
	url := "https://parent.example.com/s/abc123"
	in := MessageInput{
		ContactName: "Chị Lan",
		PeriodLabel: "08/2025",
		Children: []ChildSummary{
			{StudentName: "An", BillableCount: 8, AbsentCount: 0, Amount: 800000},
			{StudentName: "Bình", BillableCount: 6, AbsentCount: 2, Amount: 600000},
		},
		TotalDue: 1400000,
		URL:      url,
	}

	text, collapsed := Build(in, 1000)

	if collapsed {
		t.Fatal("expected no collapse")
	}
	if want := loadGolden(t, "two_children"); text != want {
		t.Fatalf("text mismatch:\ngot:  %q\nwant: %q", text, want)
	}
	assertURLLast(t, text, url)
}

func TestBuildOldDebtPresent(t *testing.T) {
	url := "https://parent.example.com/s/abc123"
	in := MessageInput{
		ContactName:    "Chị Lan",
		PeriodLabel:    "08/2025",
		Children:       []ChildSummary{{StudentName: "An", BillableCount: 8, AbsentCount: 0, Amount: 800000}},
		OpeningBalance: 200000,
		TotalDue:       1000000,
		URL:            url,
	}

	text, _ := Build(in, 1000)

	if want := loadGolden(t, "old_debt_present"); text != want {
		t.Fatalf("text mismatch:\ngot:  %q\nwant: %q", text, want)
	}
	if !strings.Contains(text, "Nợ cũ: 200.000 đ") {
		t.Fatal("expected old debt line present")
	}
	assertURLLast(t, text, url)
}

func TestBuildOldDebtZeroOmitsLine(t *testing.T) {
	url := "https://parent.example.com/s/abc123"
	in := MessageInput{
		ContactName:    "Anh Minh",
		PeriodLabel:    "08/2025",
		Children:       []ChildSummary{{StudentName: "Chi", BillableCount: 4, AbsentCount: 0, Amount: 400000}},
		OpeningBalance: 0,
		TotalDue:       400000,
		URL:            url,
	}

	text, _ := Build(in, 1000)

	if want := loadGolden(t, "old_debt_zero"); text != want {
		t.Fatalf("text mismatch:\ngot:  %q\nwant: %q", text, want)
	}
	if strings.Contains(text, "Nợ cũ") {
		t.Fatal("expected no old debt line when opening balance is zero")
	}
	assertURLLast(t, text, url)
}

func TestBuildNegativeAdjustment(t *testing.T) {
	url := "https://parent.example.com/s/abc123"
	in := MessageInput{
		ContactName:     "Chị Lan",
		PeriodLabel:     "08/2025",
		Children:        []ChildSummary{{StudentName: "An", BillableCount: 8, AbsentCount: 0, Amount: 800000}},
		AdjustmentTotal: -50000,
		TotalDue:        750000,
		URL:             url,
	}

	text, _ := Build(in, 1000)

	if want := loadGolden(t, "negative_adjustment"); text != want {
		t.Fatalf("text mismatch:\ngot:  %q\nwant: %q", text, want)
	}
	if !strings.Contains(text, "Điều chỉnh: -50.000 đ") {
		t.Fatal("expected negative adjustment sign preserved")
	}
	assertURLLast(t, text, url)
}

func TestBuildPositiveAdjustment(t *testing.T) {
	url := "https://parent.example.com/s/abc123"
	in := MessageInput{
		ContactName:     "Chị Lan",
		PeriodLabel:     "08/2025",
		Children:        []ChildSummary{{StudentName: "An", BillableCount: 8, AbsentCount: 0, Amount: 800000}},
		AdjustmentTotal: 50000,
		TotalDue:        850000,
		URL:             url,
	}

	text, _ := Build(in, 1000)

	if want := loadGolden(t, "positive_adjustment"); text != want {
		t.Fatalf("text mismatch:\ngot:  %q\nwant: %q", text, want)
	}
	if !strings.Contains(text, "Điều chỉnh: +50.000 đ") {
		t.Fatal("expected explicit plus sign on positive adjustment")
	}
	assertURLLast(t, text, url)
}

func fiveChildrenInput(url string) MessageInput {
	return MessageInput{
		ContactName: "Chị Lan",
		PeriodLabel: "08/2025",
		Children: []ChildSummary{
			{StudentName: "An", BillableCount: 8, AbsentCount: 0, Amount: 800000},
			{StudentName: "Bình", BillableCount: 6, AbsentCount: 2, Amount: 600000},
			{StudentName: "Chi", BillableCount: 8, AbsentCount: 0, Amount: 800000},
			{StudentName: "Dũng", BillableCount: 7, AbsentCount: 1, Amount: 700000},
			{StudentName: "Em", BillableCount: 8, AbsentCount: 0, Amount: 800000},
		},
		TotalDue: 3700000,
		URL:      url,
	}
}

func TestBuildUnderMaxLenStaysUncollapsed(t *testing.T) {
	url := "https://parent.example.com/s/abc123"
	in := fiveChildrenInput(url)

	text, collapsed := Build(in, 1000)

	if collapsed {
		t.Fatal("expected no collapse when text fits maxLen")
	}
	if want := loadGolden(t, "collapse_full"); text != want {
		t.Fatalf("text mismatch:\ngot:  %q\nwant: %q", text, want)
	}
	assertURLLast(t, text, url)
}

func TestBuildOverMaxLenCollapses(t *testing.T) {
	url := "https://parent.example.com/s/abc123"
	in := fiveChildrenInput(url)

	full, _ := Build(in, 1000)
	text, collapsed := Build(in, 150)

	if !collapsed {
		t.Fatal("expected collapse when text exceeds maxLen")
	}
	if len(full) <= 150 {
		t.Fatalf("test setup invalid: full text must exceed maxLen, got len %d", len(full))
	}
	if want := loadGolden(t, "collapsed"); text != want {
		t.Fatalf("text mismatch:\ngot:  %q\nwant: %q", text, want)
	}
	if !strings.Contains(text, "5 bạn, tổng 37 buổi học") {
		t.Fatal("expected collapsed per-child summary line")
	}
	// Old debt, total, and URL must all survive collapse.
	if !strings.Contains(text, "Tổng cộng: 3.700.000 đ") {
		t.Fatal("expected total line to survive collapse")
	}
	assertURLLast(t, text, url)
}

func TestBuildCollapsedFormStillContainsURL(t *testing.T) {
	url := "https://parent.example.com/s/abc123"
	in := fiveChildrenInput(url)

	text, collapsed := Build(in, 150)

	if !collapsed {
		t.Fatal("expected collapse")
	}
	if !strings.Contains(text, url) {
		t.Fatal("expected url to survive collapse")
	}
	assertURLLast(t, text, url)
}

func TestFormatMoneyGrouping(t *testing.T) {
	cases := map[int64]string{
		0:       "0 đ",
		800000:  "800.000 đ",
		1000:    "1.000 đ",
		1234567: "1.234.567 đ",
		-50000:  "-50.000 đ",
	}
	for amount, want := range cases {
		if got := formatMoney(amount); got != want {
			t.Errorf("formatMoney(%d) = %q, want %q", amount, got, want)
		}
	}
}

func TestFormatSignedMoney(t *testing.T) {
	if got := formatSignedMoney(50000); got != "+50.000 đ" {
		t.Errorf("formatSignedMoney(50000) = %q", got)
	}
	if got := formatSignedMoney(-50000); got != "-50.000 đ" {
		t.Errorf("formatSignedMoney(-50000) = %q", got)
	}
}
