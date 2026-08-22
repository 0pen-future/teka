package imports

import (
	"fmt"
	"net/http"

	"teka/apps/api/internal/shared/apperror"
)

// Row-level error codes. Each names something the operator can fix in Excel
// and re-upload. Codes owned by the parser live here; resolution and write
// phases add their own to this list.
const (
	// Parser codes.
	CodeMissingRequired = "MISSING_REQUIRED"
	CodeBadFormat       = "BAD_FORMAT"
	CodeTooLong         = "TOO_LONG"

	// CodeEmptyFile is a whole-file rejection: the workbook parsed and resolved
	// to no classes and no students. It is not a row defect — there is no row —
	// so it rides the top-level error code, not the per-row list.
	CodeEmptyFile = "EMPTY_FILE"
)

// emptyFileErr is the 422 returned when a workbook carries no data rows at all.
// The most common cause is data typed on row 2 (the example row, skipped by
// position) or an untouched template, so the message names where data must
// start. It is a distinct code from the row-defect 422 so a client can tell
// "nothing to import" apart from "some rows are wrong".
func emptyFileErr() *apperror.AppError {
	return apperror.New(CodeEmptyFile, http.StatusUnprocessableEntity,
		"file không có dòng dữ liệu nào — nhập từ dòng 3 trở đi")
}

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
