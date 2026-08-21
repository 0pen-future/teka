// Package imports turns an operator-supplied Excel workbook into classes,
// schedules, parent contacts, students, and enrollments. This file is the
// single source of truth for the workbook's shape: sheet names, column
// headers, value vocabularies, and length caps. template.go writes headers
// from here and parser.go verifies against here, so the two cannot drift.
package imports

// Sheet names, matched exactly. Renaming one is a whole-file error, never a
// row error — the operator has the wrong file, not a bad row.
const (
	SheetClasses  = "Lop"
	SheetStudents = "HocSinh"
)

// MaxRowsPerSheet bounds the work a single upload can trigger. It is enforced
// by the streaming row iterator, before the sheet is materialised, so a small
// but highly compressible workbook cannot expand into memory first.
//
// 500 is the number the operator asked for, and the write path was measured
// against it: a full 500-class/500-student workbook commits in ~1.3s and
// re-imports in ~0.5s. That is against a local Postgres, where a round trip is
// far cheaper than in production, and the commit issues roughly ten per
// student row — so the real figure is higher. The headroom to the server's
// WriteTimeout (30s, server.go) and to the browser client's 10s is still
// wide enough that the cap is a guard against abuse, not a latency budget.
// Re-measure before raising it.
const MaxRowsPerSheet = 500

// Row 1 is the header and row 2 is a filled-in example the parser skips, so
// real data starts at row 3.
const (
	headerRow      = 1
	exampleRow     = 2
	firstDataRow   = 3
	defaultDuraMin = 90
)

// Length caps mirror the binding tags on the four CreateRequest DTOs, which in
// turn mirror the VARCHAR widths. The services are called service-to-service
// here, below the gin binding layer that normally enforces them, so a value
// that overruns would surface as a mid-transaction 22001 with no line number.
// columns_test.go pins these against the DTO tags.
const (
	MaxClassName   = 100 // classes.CreateClassRequest.Name
	MaxFullName    = 100 // students.CreateRequest.FullName, contacts.CreateRequest.FullName
	MaxDisplayNote = 50  // students.CreateRequest.DisplayNote
)

// Column headers, in order. The index of each name in the slice is its column
// position; parser.go reads cells positionally after verifying the header row.
var classHeaders = []string{
	"Tên lớp",
	"SĐT giáo viên",
	"Ngày khai giảng (dd/mm/yyyy)",
	"Đơn giá/buổi (đồng)",
	"Thứ (2-7 hoặc CN)",
	"Giờ bắt đầu (HH:MM)",
	"Thời lượng (phút)",
	"Ngày kết thúc (dd/mm/yyyy)",
}

// Column indices into classHeaders.
const (
	colClassName = iota
	colClassTeacherPhone
	colClassStartDate
	colClassUnitPrice
	colClassWeekday
	colClassStartTime
	colClassDuration
	colClassEndDate
)

var studentHeaders = []string{
	"Họ tên học sinh",
	"Họ tên phụ huynh",
	"SĐT phụ huynh",
	"Tên lớp",
	"SĐT giáo viên",
	"Ngày nhập học (dd/mm/yyyy)",
	"Ghi chú phân biệt",
}

// Column indices into studentHeaders.
const (
	colStudentName = iota
	colContactName
	colContactPhone
	colStudentClassName
	colStudentTeacherPhone
	colStudentStartedOn
	colStudentDisplayNote
)

// weekdayFromSheet maps the Vietnamese day names an operator writes onto
// time.Weekday, where Sunday is 0 — the same convention class_schedules.weekday
// stores. "Thứ N" is N-1; Chủ nhật is 0, never 8.
var weekdayFromSheet = map[string]int16{
	"CN": 0,
	"2":  1,
	"3":  2,
	"4":  3,
	"5":  4,
	"6":  5,
	"7":  6,
}

// exampleClassRow and exampleStudentRow are written into row 2 of the
// generated template so the operator sees a filled-in shape. The parser skips
// row 2 by position, so these values never reach the database.
var exampleClassRow = []string{
	"Toán 9A", "0912345678", "01/09/2025", "150000", "2", "18:00", "90", "",
}

var exampleStudentRow = []string{
	"Phạm Gia An", "Phạm Văn Hùng", "0901234567", "Toán 9A", "0912345678", "", "",
}
