package imports

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// Unzip limits for OpenReader. UnzipSizeLimit is a real ceiling: excelize
// refuses a workbook whose declared uncompressed size exceeds it.
// UnzipXMLSizeLimit is NOT a rejection limit — excelize spills worksheet XML
// past it to a temp file and carries on — so it bounds memory, not work. The
// upload byte cap and MaxRowsPerSheet are what actually bound a decompression
// bomb; these two keep one sheet from being held in memory whole.
const (
	unzipSizeLimit    = 32 << 20 // 32 MiB total
	unzipXMLSizeLimit = 8 << 20  // 8 MiB per worksheet
)

// ClassRow is one line of the Lop sheet: a single weekly slot. Several rows
// sharing a class name and teacher describe one class with several slots;
// grouping happens during resolution, not here.
type ClassRow struct {
	Line         int
	Name         string
	TeacherPhone string // E.164, empty when the cell was blank (means: the owner)
	StartDate    time.Time
	UnitPrice    int64
	Weekday      int16
	StartTime    string // "HH:MM"
	DurationMin  int16
	EndDate      *time.Time
}

// StudentRow is one line of the HocSinh sheet: one student in one class.
type StudentRow struct {
	Line         int
	StudentName  string
	ContactName  string
	ContactPhone string // E.164
	ClassName    string
	TeacherPhone string // E.164, empty when the cell was blank (means: the owner)
	StartedOn    *time.Time
	DisplayNote  string // empty means unset; stored as NULL
}

// ParsedWorkbook holds coerced but still unresolved rows — no uuid has been
// looked up and no cross-sheet reference has been checked.
type ParsedWorkbook struct {
	Classes  []ClassRow
	Students []StudentRow
}

// ParseWorkbook coerces both sheets and collects every row-level defect it
// finds. It deliberately does not stop at the first bad row: an operator
// fixing a 300-row roster needs the whole list in one pass, not one error per
// upload.
//
// A defect in the workbook's structure — a missing sheet, a renamed header,
// more rows than the cap — comes back as an error instead, because there is
// no row to point at and nothing to partially salvage.
func ParseWorkbook(b []byte) (*ParsedWorkbook, []RowError, error) {
	if err := checkWorksheetsIntact(b); err != nil {
		return nil, nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(b), excelize.Options{
		UnzipSizeLimit:    unzipSizeLimit,
		UnzipXMLSizeLimit: unzipXMLSizeLimit,
	})
	if err != nil {
		return nil, nil, fileErrf("không đọc được file: phải là tệp Excel .xlsx hợp lệ")
	}
	defer func() { _ = f.Close() }()

	var rowErrs []RowError

	classRaw, err := readSheet(f, SheetClasses, classHeaders)
	if err != nil {
		return nil, nil, err
	}
	studentRaw, err := readSheet(f, SheetStudents, studentHeaders)
	if err != nil {
		return nil, nil, err
	}

	wb := &ParsedWorkbook{
		Classes:  make([]ClassRow, 0, len(classRaw)),
		Students: make([]StudentRow, 0, len(studentRaw)),
	}
	for _, raw := range classRaw {
		row, errs := coerceClassRow(raw)
		rowErrs = append(rowErrs, errs...)
		if len(errs) == 0 {
			wb.Classes = append(wb.Classes, row)
		}
	}
	for _, raw := range studentRaw {
		row, errs := coerceStudentRow(raw)
		rowErrs = append(rowErrs, errs...)
		if len(errs) == 0 {
			wb.Students = append(wb.Students, row)
		}
	}
	return wb, rowErrs, nil
}

// checkWorksheetsIntact verifies every worksheet part is well-formed XML.
//
// This is not paranoia about malformed files, it is a correctness check
// excelize cannot do for us: its row iterator discards decoder errors, so a
// worksheet whose XML is cut mid-stream simply stops early and iter.Error()
// stays nil. A truncated 300-row roster would import as 40 rows and report
// success — and this feature is all-or-nothing, so a silently partial import
// is the worst outcome it has. The declared <dimension> is no help either:
// excelize leaves it at "A1" for workbooks written cell by cell.
func checkWorksheetsIntact(b []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		// Not a zip at all; excelize reports this better than we can.
		return nil
	}
	for _, part := range zr.File {
		if !strings.HasPrefix(part.Name, "xl/worksheets/") || !strings.HasSuffix(part.Name, ".xml") {
			continue
		}
		rc, err := part.Open()
		if err != nil {
			return fileErrf("file Excel hỏng: không đọc được dữ liệu bảng tính")
		}
		dec := xml.NewDecoder(rc)
		for {
			_, err = dec.Token()
			if err != nil {
				break
			}
		}
		_ = rc.Close()
		if !errors.Is(err, io.EOF) {
			return fileErrf("file Excel hỏng hoặc tải lên chưa xong; hãy tải lại file")
		}
	}
	return nil
}

// rawRow is one worksheet row with its own line number, already padded to the
// header width so a caller can index any column without a bounds check.
type rawRow struct {
	line  int
	cells []string
}

// readSheet streams one sheet through excelize's row iterator, verifies the
// header, skips the example row, and stops at the cap.
//
// Two choices here are load-bearing:
//
// Cells are read RAW, not as display strings. A display string is rendered
// through the cell's number format, so a genuine Excel date cell shows
// "09/01/2025" under mm/dd/yyyy and "01/09/2025" under dd/mm/yyyy — the same
// day, eight months apart once parsed as dd/mm, with nothing to warn on.
// Reading raw turns such a cell into its serial ("45901"), which the date
// coercion rejects visibly and explains.
//
// Rows stream rather than being materialised. GetRows would decompress the
// entire sheet into [][]string before returning, so a row cap applied
// afterwards would protect nothing; here the cap is checked as rows arrive.
func readSheet(f *excelize.File, sheet string, headers []string) ([]rawRow, error) {
	if !sheetExists(f, sheet) {
		return nil, fileErrf("file thiếu sheet %q", sheet)
	}
	iter, err := f.Rows(sheet)
	if err != nil {
		return nil, fileErrf("không đọc được sheet %q", sheet)
	}
	defer func() { _ = iter.Close() }()

	var out []rawRow
	line := 0
	headerSeen := false
	for iter.Next() {
		line++
		cells, err := iter.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, fileErrf("không đọc được dòng %d của sheet %q", line, sheet)
		}
		switch line {
		case headerRow:
			if err := checkHeader(sheet, cells, headers); err != nil {
				return nil, err
			}
			headerSeen = true
			continue
		case exampleRow:
			// Row 2 of the generated template is a filled-in example. Skipping
			// it by position needs no in-band marker in the data itself.
			continue
		}
		// Blankness is judged on the contract's columns only. A note the
		// operator left in some far-off cell must not resurrect an empty row
		// and turn it into a wall of MISSING_REQUIRED errors.
		if isBlank(cells[:min(len(cells), len(headers))]) {
			continue
		}
		if len(out) >= MaxRowsPerSheet {
			return nil, fileErrf("sheet %q vượt quá %d dòng dữ liệu", sheet, MaxRowsPerSheet)
		}
		out = append(out, rawRow{line: line, cells: pad(cells, len(headers))})
	}
	if err := iter.Error(); err != nil {
		return nil, fileErrf("không đọc được sheet %q", sheet)
	}
	if !headerSeen {
		// An empty sheet never enters the loop, so the header check above
		// never runs — the most likely wrong file would otherwise import as a
		// silent success.
		return nil, fileErrf("sheet %q trống, thiếu dòng tiêu đề", sheet)
	}
	return out, nil
}

// sheetExists reports whether the workbook has a sheet with exactly this name.
// GetSheetIndex cannot do this check: it returns (-1, nil) rather than an
// error for an absent sheet, and it matches case-insensitively, so "LOP" would
// pass a contract that says the name is exact.
func sheetExists(f *excelize.File, sheet string) bool {
	for _, name := range f.GetSheetList() {
		if name == sheet {
			return true
		}
	}
	return false
}

// checkHeader compares the header row against the contract in columns.go. A
// renamed, reordered or appended column is a whole-file error: every
// subsequent read is positional, so continuing would silently write the wrong
// column's value — or, for an appended column, silently discard whatever the
// operator put under it.
func checkHeader(sheet string, cells, headers []string) error {
	got := pad(cells, len(headers))
	for i, want := range headers {
		if trimCell(got[i]) != want {
			return fileErrf("sheet %q sai tiêu đề ở cột %d: cần %q, đang là %q",
				sheet, i+1, want, trimCell(got[i]))
		}
	}
	for i := len(headers); i < len(cells); i++ {
		if extra := trimCell(cells[i]); extra != "" {
			return fileErrf("sheet %q có cột thừa ở vị trí %d (%q); import không đọc cột này nên hãy xoá nó",
				sheet, i+1, extra)
		}
	}
	return nil
}

// pad widens a row to n cells. excelize drops trailing empties, so a row whose
// last column is blank comes back short and would panic an indexed read.
func pad(cells []string, n int) []string {
	if len(cells) >= n {
		return cells[:n]
	}
	out := make([]string, n)
	copy(out, cells)
	return out
}

func isBlank(cells []string) bool {
	for _, c := range cells {
		if trimCell(c) != "" {
			return false
		}
	}
	return true
}

// coerceClassRow converts one Lop line, accumulating every defect in the line
// rather than returning at the first.
func coerceClassRow(raw rawRow) (ClassRow, []RowError) {
	var errs []RowError
	add := func(e *RowError) {
		if e != nil {
			errs = append(errs, *e)
		}
	}
	at := func(i int) string { return raw.cells[i] }
	hdr := func(i int) string { return classHeaders[i] }

	row := ClassRow{Line: raw.line}

	name := cleanName(at(colClassName))
	if _, e := required(SheetClasses, raw.line, hdr(colClassName), name); e != nil {
		add(e)
	} else if v, e := capped(SheetClasses, raw.line, hdr(colClassName), name, MaxClassName); e != nil {
		add(e)
	} else {
		row.Name = v
	}

	phone, e := parsePhone(SheetClasses, raw.line, hdr(colClassTeacherPhone), at(colClassTeacherPhone))
	add(e)
	row.TeacherPhone = phone

	if _, e := required(SheetClasses, raw.line, hdr(colClassStartDate), trimCell(at(colClassStartDate))); e != nil {
		add(e)
	} else if d, e := parseSheetDate(SheetClasses, raw.line, hdr(colClassStartDate), at(colClassStartDate)); e != nil {
		add(e)
	} else {
		row.StartDate = d
	}

	if _, e := required(SheetClasses, raw.line, hdr(colClassUnitPrice), trimCell(at(colClassUnitPrice))); e != nil {
		add(e)
	} else if p, e := parseMoney(SheetClasses, raw.line, hdr(colClassUnitPrice), at(colClassUnitPrice)); e != nil {
		add(e)
	} else {
		row.UnitPrice = p
	}

	wd, e := parseWeekday(SheetClasses, raw.line, hdr(colClassWeekday), at(colClassWeekday))
	add(e)
	row.Weekday = wd

	st, e := parseStartTime(SheetClasses, raw.line, hdr(colClassStartTime), at(colClassStartTime))
	add(e)
	row.StartTime = st

	dur, e := parseDuration(SheetClasses, raw.line, hdr(colClassDuration), at(colClassDuration))
	add(e)
	row.DurationMin = dur

	end, e := parseOptionalDate(SheetClasses, raw.line, hdr(colClassEndDate), at(colClassEndDate))
	add(e)
	row.EndDate = end

	return row, errs
}

// coerceStudentRow converts one HocSinh line.
func coerceStudentRow(raw rawRow) (StudentRow, []RowError) {
	var errs []RowError
	add := func(e *RowError) {
		if e != nil {
			errs = append(errs, *e)
		}
	}
	at := func(i int) string { return raw.cells[i] }
	hdr := func(i int) string { return studentHeaders[i] }

	row := StudentRow{Line: raw.line}

	for _, f := range []struct {
		idx   int
		limit int
		dst   *string
	}{
		{colStudentName, MaxFullName, &row.StudentName},
		{colContactName, MaxFullName, &row.ContactName},
		{colStudentClassName, MaxClassName, &row.ClassName},
	} {
		v := cleanName(at(f.idx))
		if _, e := required(SheetStudents, raw.line, hdr(f.idx), v); e != nil {
			add(e)
			continue
		}
		if v, e := capped(SheetStudents, raw.line, hdr(f.idx), v, f.limit); e != nil {
			add(e)
		} else {
			*f.dst = v
		}
	}

	if _, e := required(SheetStudents, raw.line, hdr(colContactPhone), trimCell(at(colContactPhone))); e != nil {
		add(e)
	} else if p, e := parsePhone(SheetStudents, raw.line, hdr(colContactPhone), at(colContactPhone)); e != nil {
		add(e)
	} else {
		row.ContactPhone = p
	}

	tp, e := parsePhone(SheetStudents, raw.line, hdr(colStudentTeacherPhone), at(colStudentTeacherPhone))
	add(e)
	row.TeacherPhone = tp

	started, e := parseOptionalDate(SheetStudents, raw.line, hdr(colStudentStartedOn), at(colStudentStartedOn))
	add(e)
	row.StartedOn = started

	note := cleanName(at(colStudentDisplayNote))
	if v, e := capped(SheetStudents, raw.line, hdr(colStudentDisplayNote), note, MaxDisplayNote); e != nil {
		add(e)
	} else {
		row.DisplayNote = v
	}

	return row, errs
}
