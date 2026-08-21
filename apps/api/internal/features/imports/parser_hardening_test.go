package imports

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// TestRealDateCellIsRejectedNotMisread is the regression test for the worst
// defect this parser can have. A cell Excel holds as a genuine date renders
// through its number format, so the same day reads as "09/01/2025" on an
// mm/dd sheet and "01/09/2025" on a dd/mm one. Parsed as dd/mm those are eight
// months apart, and nothing would flag it.
//
// Reading raw values turns both into the same serial, which is rejected with a
// message naming the fix.
func TestRealDateCellIsRejectedNotMisread(t *testing.T) {
	t.Parallel()
	for _, numFmt := range []string{"mm/dd/yyyy", "dd/mm/yyyy"} {
		t.Run(numFmt, func(t *testing.T) {
			t.Parallel()
			f := excelize.NewFile()
			defer func() { _ = f.Close() }()
			newSheets(t, f)
			writeRows(t, f, SheetClasses, [][]string{classHeaders, exampleClassRow,
				{"Toan 9A", "0912345678", "", "150000", "2", "18:00", "", ""}})
			writeRows(t, f, SheetStudents, [][]string{studentHeaders, exampleStudentRow})

			// C3 of the data row: a real date value, not text.
			style, err := f.NewStyle(&excelize.Style{CustomNumFmt: &numFmt})
			require.NoError(t, err)
			require.NoError(t, f.SetCellValue(SheetClasses, "C3", time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)))
			require.NoError(t, f.SetCellStyle(SheetClasses, "C3", "C3", style))

			buf, err := f.WriteToBuffer()
			require.NoError(t, err)

			wb, rowErrs, err := ParseWorkbook(buf.Bytes())
			require.NoError(t, err)
			require.Empty(t, wb.Classes, "a date-formatted cell must never parse into a date")
			require.Len(t, rowErrs, 1)
			require.Equal(t, CodeBadFormat, rowErrs[0].Code)
			// The operator sees a bare serial number, so the message has to
			// name both the cause (an Excel date cell) and the fix (Text).
			require.Contains(t, rowErrs[0].Message, "Excel")
			require.Contains(t, rowErrs[0].Message, "Text")
		})
	}
}

// TestThousandsSeparatorCellIsAccepted is the other half of reading raw: a
// number cell displayed as "150,000" carries the plain value underneath, so
// formatting the price column no longer costs the operator an error.
func TestThousandsSeparatorCellIsAccepted(t *testing.T) {
	t.Parallel()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	newSheets(t, f)
	writeRows(t, f, SheetClasses, [][]string{classHeaders, exampleClassRow,
		{"Toan 9A", "0912345678", "01/09/2025", "", "2", "18:00", "", ""}})
	writeRows(t, f, SheetStudents, [][]string{studentHeaders, exampleStudentRow})

	sep := "#,##0"
	style, err := f.NewStyle(&excelize.Style{CustomNumFmt: &sep})
	require.NoError(t, err)
	require.NoError(t, f.SetCellValue(SheetClasses, "D3", 150000))
	require.NoError(t, f.SetCellStyle(SheetClasses, "D3", "D3", style))

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)

	wb, rowErrs, err := ParseWorkbook(buf.Bytes())
	require.NoError(t, err)
	require.Empty(t, rowErrs)
	require.Len(t, wb.Classes, 1)
	require.Equal(t, int64(150000), wb.Classes[0].UnitPrice)
}

func TestEmptySheetIsRejected(t *testing.T) {
	t.Parallel()
	// An empty sheet never enters the row loop, so without an explicit check
	// the header verification never runs and the most likely wrong file -- a
	// blank one -- imports as a silent success.
	_, _, err := ParseWorkbook(buildWorkbook(t, map[string][][]string{
		SheetClasses:  {},
		SheetStudents: {studentHeaders, exampleStudentRow},
	}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "tr\u1ed1ng")
}

func TestMissingSheetNamesTheSheet(t *testing.T) {
	t.Parallel()
	// GetSheetIndex returns (-1, nil) for an absent sheet, so an err != nil
	// guard is dead code and the operator would get "khong doc duoc sheet"
	// instead of "thieu sheet".
	_, _, err := ParseWorkbook(buildWorkbook(t, map[string][][]string{
		SheetClasses: {classHeaders, exampleClassRow},
	}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "thi\u1ebfu sheet")
	require.Contains(t, err.Error(), SheetStudents)
}

func TestSheetNameIsCaseSensitive(t *testing.T) {
	t.Parallel()
	// excelize matches sheet names case-insensitively; the contract says
	// exact, so the parser does its own lookup.
	_, _, err := ParseWorkbook(buildWorkbook(t, map[string][][]string{
		"LOP":         {classHeaders, exampleClassRow},
		SheetStudents: {studentHeaders, exampleStudentRow},
	}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "thi\u1ebfu sheet")
}

func TestAppendedColumnIsRejectedNotDropped(t *testing.T) {
	t.Parallel()
	// "Don gia rieng" is a deliberate scope cut, so it is exactly the column an
	// operator will re-add. Silently truncating it would replace every
	// per-student price with the class default and say nothing.
	extra := "\u0110\u01a1n gi\u00e1 ri\u00eang"
	extraHeader := append(append([]string(nil), studentHeaders...), extra)
	extraRow := []string{"A", "B", "0901234567", "Toan 9A", "0912345678", "", "", "130000"}

	_, _, err := ParseWorkbook(buildWorkbook(t, map[string][][]string{
		SheetClasses:  {classHeaders, exampleClassRow},
		SheetStudents: {extraHeader, exampleStudentRow, extraRow},
	}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "c\u1ed9t th\u1eeba")
	require.Contains(t, err.Error(), extra)
}

func TestStrayCellOutsideContractDoesNotResurrectBlankRows(t *testing.T) {
	t.Parallel()
	// A leftover note in a far-off column must not turn an empty row into a
	// wall of MISSING_REQUIRED errors that block an otherwise valid roster.
	rows := [][]string{classHeaders, exampleClassRow,
		{"Toan 9A", "0912345678", "01/09/2025", "150000", "2", "18:00", "", ""},
		{"", "", "", "", "", "", "", "", "", "ghi chu cu cua toi"},
	}
	wb, rowErrs, err := ParseWorkbook(buildWorkbook(t, map[string][][]string{
		SheetClasses:  rows,
		SheetStudents: {studentHeaders, exampleStudentRow},
	}))
	require.NoError(t, err)
	require.Empty(t, rowErrs)
	require.Len(t, wb.Classes, 1)
}

func TestTruncatedSheetXMLIsRejected(t *testing.T) {
	t.Parallel()
	// excelize's row iterator swallows XML decoder errors, so a sheet cut
	// mid-stream just ends early with iter.Error() nil. Without the dimension
	// cross-check a 10-class roster would import as 4 and report success.
	rows := [][]string{classHeaders, exampleClassRow}
	for i := range 10 {
		rows = append(rows, []string{
			"Lop " + string(rune('A'+i)), "0912345678", "01/09/2025", "150000", "2", "18:00", "", "",
		})
	}
	full := buildWorkbook(t, map[string][][]string{
		SheetClasses:  rows,
		SheetStudents: {studentHeaders, exampleStudentRow},
	})

	wb, rowErrs, err := ParseWorkbook(full)
	require.NoError(t, err)
	require.Empty(t, rowErrs)
	require.Len(t, wb.Classes, 10, "sanity: the intact workbook has all ten")

	_, _, err = ParseWorkbook(truncateSheetXML(t, full))
	require.Error(t, err, "a short read must not pass as a short roster")
}

// truncateSheetXML rewrites the workbook's zip, cutting the largest worksheet
// in half while leaving its declared dimension intact. The sheet is picked by
// size rather than by name: excelize numbers worksheet parts by creation
// order, and buildWorkbook deletes the default sheet, so there is no
// sheet1.xml to target.
func truncateSheetXML(t *testing.T, workbook []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(workbook), int64(len(workbook)))
	require.NoError(t, err)

	var target string
	var largest uint64
	for _, file := range zr.File {
		if strings.HasPrefix(file.Name, "xl/worksheets/sheet") && file.UncompressedSize64 > largest {
			target, largest = file.Name, file.UncompressedSize64
		}
	}
	require.NotEmpty(t, target, "no worksheet part found in the workbook")

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range zr.File {
		rc, err := file.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, rc.Close())
		require.NoError(t, err)

		if file.Name == target {
			data = data[:len(data)/2]
		}
		w, err := zw.Create(file.Name)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return out.Bytes()
}

func TestInvisibleCharactersAreStrippedFromNames(t *testing.T) {
	t.Parallel()
	// Copy-paste from a web page or Word carries zero-width codepoints and
	// stray control characters. They survive trimming and NFC, are invisible
	// everywhere a human would look, and would silently break every
	// natural-key match in the write phase.
	const want = "To\u00e1n 9A"
	require.Equal(t, want, cleanName("To\u00e1n\u200b 9A"), "zero-width space")
	require.Equal(t, want, cleanName("\ufeffTo\u00e1n 9A"), "byte order mark")
	require.Equal(t, want, cleanName("To\u00e1n\u200d 9A"), "zero-width joiner")
	require.Equal(t, want, cleanName("  To\u00e1n 9A  "))
	require.Equal(t, "L\u00ea An", cleanName("L\u00ea\nAn"),
		"a newline collapses to a space, not to nothing")
}

func TestImplausibleYearIsRejected(t *testing.T) {
	t.Parallel()
	// time.Parse accepts any four-digit year and classes/dto.go validates only
	// the layout, so a mistyped year would create a class centuries away with
	// nothing downstream to catch it.
	for _, in := range []string{"01/09/0225", "01/09/2205", "01/09/1899"} {
		_, rerr := parseSheetDate(SheetClasses, 3, "Ngay", in)
		require.NotNil(t, rerr, "input %q", in)
		require.Contains(t, rerr.Message, "kh\u00f4ng h\u1ee3p l\u00fd")
	}
	_, rerr := parseSheetDate(SheetClasses, 3, "Ngay", "01/09/2025")
	require.Nil(t, rerr)
}

// newSheets adds both contract sheets and drops excelize's default one.
func newSheets(t *testing.T, f *excelize.File) {
	t.Helper()
	for _, name := range []string{SheetClasses, SheetStudents} {
		_, err := f.NewSheet(name)
		require.NoError(t, err)
	}
	require.NoError(t, f.DeleteSheet("Sheet1"))
}

// writeRows fills a sheet from row 1 down.
func writeRows(t *testing.T, f *excelize.File, sheet string, rows [][]string) {
	t.Helper()
	for r, row := range rows {
		for c, v := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+1)
			require.NoError(t, err)
			require.NoError(t, f.SetCellStr(sheet, cell, v))
		}
	}
}
