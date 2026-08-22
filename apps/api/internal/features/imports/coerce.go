package imports

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"teka/apps/api/internal/shared/validation"
)

// sheetDateLayout parses both "01/09/2025" and "1/9/2025": Go's "2" and "1"
// verbs accept one or two digits, while "02"/"01" would demand exactly two.
// An ISO date ("2025-09-01") deliberately fails — silently accepting a second
// spelling is how a workbook ends up with two date conventions in one column.
const sheetDateLayout = "2/1/2006"

// Plausibility window for a parsed year. time.Parse happily accepts 0225 and
// 2205; nothing downstream would catch either.
const (
	minPlausibleYear = 2000
	maxPlausibleYear = 2100
)

// vnPhonePattern and hhmmPattern mirror the `vnphone` and `hhmm` binding
// validators (shared/validation/validation.go:20,25). The parser feeds the
// four feature services directly, below the gin binding layer, so it has to
// enforce the same shapes itself.
var (
	vnPhonePattern = regexp.MustCompile(`^(0|\+84)(3|5|7|8|9)\d{8}$`)
	hhmmPattern    = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	digitsOnly     = regexp.MustCompile(`^\d+$`)
	looseTime      = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
	phoneNoise     = strings.NewReplacer(" ", "", " ", "", ".", "", "-", "", "(", "", ")", "")
)

// trimCell strips the whitespace Excel sprinkles into cells, including the
// non-breaking space some locales emit as a thousands separator.
func trimCell(s string) string {
	return strings.Trim(s, " \t\r\n ")
}

// cleanName strips invisible junk, trims, then NFC-normalises. Postgres
// compares VARCHAR bytewise, so the decomposed form macOS Excel writes and the
// composed form the web UI writes are two different classes unless they are
// folded here — which would make every natural-key lookup in the write phase
// miss. Control characters and zero-width codepoints survive both trimming and
// NFC, and would do the same damage while being invisible in the spreadsheet,
// in the app, and in a Zalo message.
func cleanName(s string) string {
	return norm.NFC.String(trimCell(stripInvisible(s)))
}

// stripInvisible removes control characters and zero-width formatting
// codepoints, which arrive via copy-paste from web pages and Word.
func stripInvisible(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t', r == '\n', r == '\r':
			// Whitespace inside a name collapses to a space rather than being
			// dropped, so "Lê\nAn" does not become "LêAn".
			return ' '
		case unicode.IsControl(r), r == '\u200b', r == '\u200c', r == '\u200d', r == '\ufeff':
			return -1
		default:
			return r
		}
	}, s)
}

// capped reports a TOO_LONG row error when a cleaned value overruns the column
// it is bound for. Counting runes, not bytes: the caps mirror VARCHAR(n),
// which Postgres also counts in characters.
func capped(sheet string, line int, column, value string, limit int) (string, *RowError) {
	if len([]rune(value)) > limit {
		e := rowErr(sheet, line, column, CodeTooLong,
			"tối đa %d ký tự, ô này có %d", limit, len([]rune(value)))
		return "", &e
	}
	return value, nil
}

// required rejects an empty cell.
func required(sheet string, line int, column, value string) (string, *RowError) {
	if value == "" {
		e := rowErr(sheet, line, column, CodeMissingRequired, "không được để trống")
		return "", &e
	}
	return value, nil
}

// parsePhone cleans formatting noise, checks the Vietnamese mobile shape, and
// returns the E.164 storage form. NormalizePhone alone is a prefix swap that
// validates nothing and leaves "84912345678" untouched, so the pattern check
// has to happen here or a malformed number reaches the database.
func parsePhone(sheet string, line int, column, raw string) (string, *RowError) {
	v := phoneNoise.Replace(trimCell(raw))
	if v == "" {
		return "", nil
	}
	if !vnPhonePattern.MatchString(v) {
		e := rowErr(sheet, line, column, CodeBadFormat,
			"số điện thoại phải dạng 0xxxxxxxxx hoặc +84xxxxxxxxx")
		return "", &e
	}
	return validation.NormalizePhone(v), nil
}

// parseMoney accepts a bare integer number of đồng. Every thousands separator
// spelling is rejected rather than guessed: "150.000" is 150000 in Vietnam and
// 150.0 elsewhere, and picking one silently would misprice a class.
func parseMoney(sheet string, line int, column, raw string) (int64, *RowError) {
	v := strings.ReplaceAll(trimCell(raw), " ", "")
	if !digitsOnly.MatchString(v) {
		e := rowErr(sheet, line, column, CodeBadFormat,
			"phải là số nguyên đồng, không dấu chấm/phẩy và không hậu tố (ví dụ 150000)")
		return 0, &e
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		e := rowErr(sheet, line, column, CodeBadFormat, "số tiền quá lớn")
		return 0, &e
	}
	return n, nil
}

// parseSheetDate parses dd/mm/yyyy. time.Parse rejects an out-of-range day for
// the month, so "31/02/2025" fails here rather than rolling into March.
//
// Cells are read raw, so a cell Excel holds as a real date arrives as its
// serial number rather than as text. That is deliberate — a date cell's
// displayed text depends on its number format, and reading the display would
// silently turn 1 September into 9 January on a mm/dd-formatted sheet. The
// serial is detected here only to explain the fix to the operator.
func parseSheetDate(sheet string, line int, column, raw string) (time.Time, *RowError) {
	v := trimCell(raw)
	t, err := time.Parse(sheetDateLayout, v)
	if err != nil {
		msg := "ngày phải dạng dd/mm/yyyy (ví dụ 01/09/2025)"
		if isExcelDateSerial(v) {
			msg = "ô này đang ở định dạng Ngày của Excel; hãy đổi định dạng cột sang Text rồi gõ lại dd/mm/yyyy"
		}
		e := rowErr(sheet, line, column, CodeBadFormat, "%s", msg)
		return time.Time{}, &e
	}
	if t.Year() < minPlausibleYear || t.Year() > maxPlausibleYear {
		// A mistyped year is invisible downstream: classes/dto.go validates the
		// layout only, so "01/09/0225" would create a class 1800 years off.
		e := rowErr(sheet, line, column, CodeBadFormat,
			"năm %d không hợp lý; kiểm tra lại ô này", t.Year())
		return time.Time{}, &e
	}
	return t, nil
}

// isExcelDateSerial reports whether v looks like an Excel date serial rather
// than a typo. The window spans roughly 1990-2090, which covers every date a
// tuition roster can legitimately carry.
func isExcelDateSerial(v string) bool {
	if !digitsOnly.MatchString(v) {
		return false
	}
	n, err := strconv.Atoi(v)
	return err == nil && n >= 32874 && n <= 69400
}

// parseOptionalDate is parseSheetDate for a cell that may be blank.
func parseOptionalDate(sheet string, line int, column, raw string) (*time.Time, *RowError) {
	if trimCell(raw) == "" {
		return nil, nil
	}
	t, rerr := parseSheetDate(sheet, line, column, raw)
	if rerr != nil {
		return nil, rerr
	}
	return &t, nil
}

// parseWeekday maps the sheet's day vocabulary onto time.Weekday.
func parseWeekday(sheet string, line int, column, raw string) (int16, *RowError) {
	v := strings.ToUpper(trimCell(raw))
	wd, ok := weekdayFromSheet[v]
	if !ok {
		e := rowErr(sheet, line, column, CodeBadFormat, "thứ phải là 2, 3, 4, 5, 6, 7 hoặc CN")
		return 0, &e
	}
	return wd, nil
}

// parseStartTime accepts "8:00" as well as "08:00" and returns the zero-padded
// "HH:MM" form the hhmm validator and the TIME column both expect. "18:0" and
// "25:00" fail.
func parseStartTime(sheet string, line int, column, raw string) (string, *RowError) {
	v := trimCell(raw)
	if m := looseTime.FindStringSubmatch(v); m != nil {
		v = fmt.Sprintf("%02s:%s", m[1], m[2])
	}
	if !hhmmPattern.MatchString(v) {
		e := rowErr(sheet, line, column, CodeBadFormat, "giờ phải dạng HH:MM 24 giờ (ví dụ 18:00)")
		return "", &e
	}
	return v, nil
}

// parseDuration reads the lesson length in minutes; a blank cell means the
// 90-minute default the class_schedules column also defaults to.
func parseDuration(sheet string, line int, column, raw string) (int16, *RowError) {
	v := trimCell(raw)
	if v == "" {
		return defaultDuraMin, nil
	}
	if !digitsOnly.MatchString(v) {
		e := rowErr(sheet, line, column, CodeBadFormat, "thời lượng phải là số phút nguyên dương")
		return 0, &e
	}
	// ParseInt with bitSize 16 bounds the value at parse time, so the int16 the
	// column stores can never be reached through an overflowing conversion.
	n, err := strconv.ParseInt(v, 10, 16)
	if err != nil || n <= 0 || n > 24*60 {
		e := rowErr(sheet, line, column, CodeBadFormat, "thời lượng phải trong khoảng 1–1440 phút")
		return 0, &e
	}
	return int16(n), nil
}
