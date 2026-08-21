package imports

import "fmt"

// Row-level error codes. Each names something the operator can fix in Excel
// and re-upload. Codes owned by the parser live here; resolution and write
// phases add their own to this list.
const (
	// Parser codes.
	CodeMissingRequired = "MISSING_REQUIRED"
	CodeBadFormat       = "BAD_FORMAT"
	CodeTooLong         = "TOO_LONG"
)

// RowError points at one cell the operator must fix. Line is the worksheet's
// own 1-based row number so it can be read straight off the Excel gutter.
type RowError struct {
	Sheet   string `json:"sheet"`
	Line    int    `json:"line"`
	Column  string `json:"column,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// rowErr builds a RowError for one cell. Messages are Vietnamese: they are
// read by the operator, not by a developer.
func rowErr(sheet string, line int, column, code, format string, args ...any) RowError {
	return RowError{
		Sheet:   sheet,
		Line:    line,
		Column:  column,
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// FileError is a defect in the workbook as a whole — a missing sheet, a
// renamed header, more rows than the cap. There is no line to point at and no
// per-row recovery: the operator has the wrong file.
type FileError struct {
	Message string
}

func (e *FileError) Error() string { return e.Message }

func fileErrf(format string, args ...any) *FileError {
	return &FileError{Message: fmt.Sprintf(format, args...)}
}
