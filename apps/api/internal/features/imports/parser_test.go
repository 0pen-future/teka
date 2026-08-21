package imports

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// buildWorkbook writes a workbook from raw cell values so fixtures live in the
// test, not as opaque binaries in git. Rows are written starting at the header
// row, so callers supply the header and example rows explicitly.
func buildWorkbook(t *testing.T, sheets map[string][][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for name, rows := range sheets {
		_, err := f.NewSheet(name)
		require.NoError(t, err)
		for r, row := range rows {
			for c, v := range row {
				cell, err := excelize.CoordinatesToCellName(c+1, r+1)
				require.NoError(t, err)
				require.NoError(t, f.SetCellStr(name, cell, v))
			}
		}
	}
	require.NoError(t, f.DeleteSheet("Sheet1"))
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf.Bytes()
}

// validWorkbook is the roster from the template spec: two classes (one with
// two weekly slots), three students, two parents — one of whom has children
// under two different teachers.
func validWorkbook(t *testing.T) []byte {
	t.Helper()
	return buildWorkbook(t, map[string][][]string{
		SheetClasses: {
			classHeaders,
			exampleClassRow,
			{"Toán 9A", "0912345678", "01/09/2025", "150000", "2", "18:00", "90", ""},
			{"Toán 9A", "0912345678", "01/09/2025", "150000", "5", "18:00", "90", ""},
			{"Văn 8", "0987654321", "15/09/2025", "120000", "CN", "08:30", "120", ""},
		},
		SheetStudents: {
			studentHeaders,
			exampleStudentRow,
			{"Phạm Gia An", "Phạm Văn Hùng", "0901234567", "Toán 9A", "0912345678", "", ""},
			{"Phạm Gia Bảo", "Phạm Văn Hùng", "0901234567", "Văn 8", "0987654321", "", ""},
			{"Lê Thu Hà", "Lê Thị Mai", "0977888999", "Toán 9A", "0912345678", "05/10/2025", "Hà nhỏ"},
		},
	})
}

func TestParseWorkbookHappyPath(t *testing.T) {
	t.Parallel()
	wb, rowErrs, err := ParseWorkbook(validWorkbook(t))
	require.NoError(t, err)
	require.Empty(t, rowErrs)

	require.Len(t, wb.Classes, 3, "three weekly slots across two classes")
	require.Len(t, wb.Students, 3)

	// Row 2 is the example; real data starts at row 3.
	require.Equal(t, 3, wb.Classes[0].Line)
	require.Equal(t, "Toán 9A", wb.Classes[0].Name)
	require.Equal(t, "+84912345678", wb.Classes[0].TeacherPhone)
	require.Equal(t, int64(150000), wb.Classes[0].UnitPrice)
	require.Equal(t, int16(1), wb.Classes[0].Weekday, "Thứ 2 is time.Monday == 1")
	require.Equal(t, "18:00", wb.Classes[0].StartTime)
	require.Equal(t, int16(90), wb.Classes[0].DurationMin)
	require.Nil(t, wb.Classes[0].EndDate)

	require.Equal(t, int16(0), wb.Classes[2].Weekday, "CN is time.Sunday == 0")
	require.Equal(t, int16(120), wb.Classes[2].DurationMin)

	// A blank Ngày nhập học stays nil here; resolution fills it from the class.
	require.Nil(t, wb.Students[0].StartedOn)
	require.Empty(t, wb.Students[0].DisplayNote, "blank note stays empty, stored as NULL")
	require.Equal(t, "+84901234567", wb.Students[0].ContactPhone)

	require.NotNil(t, wb.Students[2].StartedOn)
	require.Equal(t, time.Date(2025, time.October, 5, 0, 0, 0, 0, time.UTC), *wb.Students[2].StartedOn)
	require.Equal(t, "Hà nhỏ", wb.Students[2].DisplayNote)
}

func TestParseWorkbookSkipsExampleAndBlankRows(t *testing.T) {
	t.Parallel()
	b := buildWorkbook(t, map[string][][]string{
		SheetClasses: {
			classHeaders,
			exampleClassRow,
			{"Toán 9A", "0912345678", "01/09/2025", "150000", "2", "18:00", "", ""},
			{"", "", "", "", "", "", "", ""},
			{"Văn 8", "", "15/09/2025", "120000", "CN", "08:30", "", ""},
		},
		SheetStudents: {studentHeaders, exampleStudentRow},
	})
	wb, rowErrs, err := ParseWorkbook(b)
	require.NoError(t, err)
	require.Empty(t, rowErrs, "the example row must not be validated as data")
	require.Len(t, wb.Classes, 2)
	require.Empty(t, wb.Classes[1].TeacherPhone, "a blank teacher phone means the owner")
	require.Equal(t, 5, wb.Classes[1].Line, "line numbers survive the blank row")
}

func TestParseWorkbookReportsEveryRowError(t *testing.T) {
	t.Parallel()
	// Four defects across two rows. The operator fixing a roster needs the
	// whole list in one pass, not one error per upload.
	b := buildWorkbook(t, map[string][][]string{
		SheetClasses: {
			classHeaders,
			exampleClassRow,
			{"", "0912345678", "31/02/2025", "150.000", "8", "18:00", "", ""},
			{"Văn 8", "0987654321", "15/09/2025", "120000", "CN", "08:30", "", ""},
		},
		SheetStudents: {studentHeaders, exampleStudentRow},
	})
	wb, rowErrs, err := ParseWorkbook(b)
	require.NoError(t, err)
	require.Len(t, wb.Classes, 1, "only the clean row survives")

	byCode := map[string]int{}
	for _, e := range rowErrs {
		require.Equal(t, SheetClasses, e.Sheet)
		require.Equal(t, 3, e.Line)
		require.NotEmpty(t, e.Column)
		require.NotEmpty(t, e.Message)
		byCode[e.Code]++
	}
	require.Equal(t, 1, byCode[CodeMissingRequired], "empty class name")
	require.Equal(t, 3, byCode[CodeBadFormat], "bad date, bad money, bad weekday")
}

func TestParseWorkbookRejectsOverlongCells(t *testing.T) {
	t.Parallel()
	long := ""
	for range MaxClassName + 1 {
		long += "a"
	}
	b := buildWorkbook(t, map[string][][]string{
		SheetClasses: {
			classHeaders,
			exampleClassRow,
			{long, "0912345678", "01/09/2025", "150000", "2", "18:00", "", ""},
		},
		SheetStudents: {studentHeaders, exampleStudentRow},
	})
	_, rowErrs, err := ParseWorkbook(b)
	require.NoError(t, err)
	require.Len(t, rowErrs, 1)
	require.Equal(t, CodeTooLong, rowErrs[0].Code, "an overlong cell is a row error, never a 22001")
	require.Equal(t, 3, rowErrs[0].Line)
}

func TestParseWorkbookRejectsStructuralDefects(t *testing.T) {
	t.Parallel()
	renamed := append([]string(nil), classHeaders...)
	renamed[colClassUnitPrice] = "Học phí"

	cases := map[string]map[string][][]string{
		"missing sheet": {
			SheetClasses: {classHeaders, exampleClassRow},
		},
		"renamed header": {
			SheetClasses:  {renamed, exampleClassRow},
			SheetStudents: {studentHeaders, exampleStudentRow},
		},
		"reordered header": {
			SheetClasses:  {{classHeaders[1], classHeaders[0]}, exampleClassRow},
			SheetStudents: {studentHeaders, exampleStudentRow},
		},
	}
	for name, sheets := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := ParseWorkbook(buildWorkbook(t, sheets))
			require.Error(t, err, "a structural defect has no row to point at")
			var fe *FileError
			require.ErrorAs(t, err, &fe)
		})
	}
}

func TestParseWorkbookRejectsNonWorkbook(t *testing.T) {
	t.Parallel()
	_, _, err := ParseWorkbook([]byte("Tên lớp,SĐT giáo viên\nToán 9A,0912345678\n"))
	require.Error(t, err, "a CSV renamed to .xlsx must not reach the coercion pass")
	var fe *FileError
	require.ErrorAs(t, err, &fe)
}

func TestParseWorkbookEnforcesRowCap(t *testing.T) {
	t.Parallel()
	rows := [][]string{classHeaders, exampleClassRow}
	for i := range MaxRowsPerSheet + 1 {
		rows = append(rows, []string{
			"Lớp " + strconv.Itoa(i), "0912345678", "01/09/2025", "150000", "2", "18:00", "", "",
		})
	}
	_, _, err := ParseWorkbook(buildWorkbook(t, map[string][][]string{
		SheetClasses:  rows,
		SheetStudents: {studentHeaders, exampleStudentRow},
	}))
	require.Error(t, err)
	var fe *FileError
	require.ErrorAs(t, err, &fe)
	require.Contains(t, fe.Message, strconv.Itoa(MaxRowsPerSheet))
}

func TestParseWorkbookHandlesRaggedRows(t *testing.T) {
	t.Parallel()
	// excelize drops trailing empty cells, so a row whose last columns are
	// blank comes back short; an indexed read would panic.
	b := buildWorkbook(t, map[string][][]string{
		SheetClasses:  {classHeaders, exampleClassRow, {"Toán 9A"}},
		SheetStudents: {studentHeaders, exampleStudentRow, {"Phạm Gia An", "Phạm Văn Hùng"}},
	})
	require.NotPanics(t, func() {
		wb, rowErrs, err := ParseWorkbook(b)
		require.NoError(t, err)
		require.Empty(t, wb.Classes)
		require.NotEmpty(t, rowErrs)
	})
}

func TestBuildTemplateRoundTrips(t *testing.T) {
	t.Parallel()
	b, err := BuildTemplate()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	// The generated template must parse cleanly with no rows: the example row
	// is skipped, so an operator who downloads and immediately re-uploads gets
	// an empty roster rather than a validation wall.
	wb, rowErrs, err := ParseWorkbook(b)
	require.NoError(t, err)
	require.Empty(t, rowErrs)
	require.Empty(t, wb.Classes)
	require.Empty(t, wb.Students)

	f, err := excelize.OpenReader(bytes.NewReader(b))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.ElementsMatch(t, []string{SheetClasses, SheetStudents}, f.GetSheetList())
}
