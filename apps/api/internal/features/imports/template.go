package imports

import (
	"strconv"

	"github.com/xuri/excelize/v2"
)

// numFmtText is Excel's built-in "@" (Text) number format. Every data column
// carries it so Excel stops "helpfully" reinterpreting the operator's input:
// without it a phone loses its leading zero, a date flips to the machine's
// mm/dd locale, and 150000 renders as 150,000.
const numFmtText = 49

// BuildTemplate produces the blank workbook the operator downloads and fills
// in: two sheets, the header row from columns.go, and one worked example on
// row 2 that the parser skips by position.
//
// Headers come from the same slices parser.go verifies against, so a typo here
// cannot produce a template the parser rejects.
func BuildTemplate() ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	textStyle, err := f.NewStyle(&excelize.Style{NumFmt: numFmtText})
	if err != nil {
		return nil, err
	}

	for _, s := range []struct {
		name    string
		headers []string
		example []string
	}{
		{SheetClasses, classHeaders, exampleClassRow},
		{SheetStudents, studentHeaders, exampleStudentRow},
	} {
		if _, err := f.NewSheet(s.name); err != nil {
			return nil, err
		}
		if err := writeSheet(f, s.name, s.headers, s.example, textStyle); err != nil {
			return nil, err
		}
	}

	// NewFile seeds a default "Sheet1"; the parser matches sheet names exactly
	// and an extra sheet is just noise in the operator's file.
	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, err
	}
	if idx, err := f.GetSheetIndex(SheetClasses); err == nil {
		f.SetActiveSheet(idx)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeSheet(f *excelize.File, sheet string, headers, example []string, textStyle int) error {
	for i, h := range headers {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return err
		}
		// Text format on the whole column, not just the filled cells: the
		// operator types into empty rows below the example.
		if err := f.SetColStyle(sheet, col, textStyle); err != nil {
			return err
		}
		if err := f.SetColWidth(sheet, col, col, columnWidth(h)); err != nil {
			return err
		}
		cell, err := excelize.CoordinatesToCellName(i+1, headerRow)
		if err != nil {
			return err
		}
		if err := f.SetCellStr(sheet, cell, h); err != nil {
			return err
		}
	}
	for i, v := range example {
		cell, err := excelize.CoordinatesToCellName(i+1, exampleRow)
		if err != nil {
			return err
		}
		if err := f.SetCellStr(sheet, cell, v); err != nil {
			return err
		}
	}
	// Freeze the header so the column names stay visible while the operator
	// scrolls a few hundred rows.
	return f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      headerRow,
		TopLeftCell: "A" + strconv.Itoa(exampleRow),
		ActivePane:  "bottomLeft",
	})
}

// columnWidth sizes a column to its header text; the headers carry the format
// hints ("(dd/mm/yyyy)") and are useless truncated.
func columnWidth(header string) float64 {
	w := float64(len([]rune(header))) + 4
	if w < 12 {
		w = 12
	}
	return w
}
