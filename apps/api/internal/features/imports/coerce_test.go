package imports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseMoney(t *testing.T) {
	t.Parallel()
	// Every thousands-separator spelling is rejected rather than guessed:
	// "150.000" is 150000 in Vietnam and 150.0 elsewhere.
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"150000", 150000, true},
		{" 150000 ", 150000, true},
		{" 150000 ", 150000, true},
		{"0", 0, true},
		{"150.000", 0, false},
		{"150,000", 0, false},
		{"150k", 0, false},
		{"1.5e5", 0, false},
		{"-1", 0, false},
		{"1.5", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, rerr := parseMoney(SheetClasses, 3, "Đơn giá", c.in)
		if c.ok {
			require.Nil(t, rerr, "input %q", c.in)
			require.Equal(t, c.want, got, "input %q", c.in)
			continue
		}
		require.NotNil(t, rerr, "input %q should be rejected", c.in)
		require.Equal(t, CodeBadFormat, rerr.Code)
	}
}

func TestParseSheetDate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in string
		ok bool
	}{
		{"01/09/2025", true},
		{"1/9/2025", true},
		{"31/12/2025", true},
		{"31/02/2025", false}, // day out of range for February
		{"2025-09-01", false}, // ISO is not a second accepted spelling
		{"09/01/2025 ", true}, // 9 January, not 1 September — dd/mm always
		{"", false},
		{"hôm nay", false},
	}
	for _, c := range cases {
		_, rerr := parseSheetDate(SheetClasses, 3, "Ngày", c.in)
		if c.ok {
			require.Nil(t, rerr, "input %q", c.in)
			continue
		}
		require.NotNil(t, rerr, "input %q should be rejected", c.in)
	}

	got, rerr := parseSheetDate(SheetClasses, 3, "Ngày", "01/09/2025")
	require.Nil(t, rerr)
	require.Equal(t, time.Date(2025, time.September, 1, 0, 0, 0, 0, time.UTC), got)
}

func TestParseWeekday(t *testing.T) {
	t.Parallel()
	// time.Weekday convention: Sunday is 0, so "Thứ N" is N-1 and CN is 0.
	for in, want := range map[string]int16{
		"CN": 0, "cn": 0, "2": 1, "3": 2, "4": 3, "5": 4, "6": 5, "7": 6,
	} {
		got, rerr := parseWeekday(SheetClasses, 3, "Thứ", in)
		require.Nil(t, rerr, "input %q", in)
		require.Equal(t, want, got, "input %q", in)
	}
	for _, in := range []string{"8", "1", "0", "", "Thứ 2", "Chủ nhật"} {
		_, rerr := parseWeekday(SheetClasses, 3, "Thứ", in)
		require.NotNil(t, rerr, "input %q should be rejected", in)
	}
}

func TestParseStartTime(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"18:00": "18:00", "8:00": "08:00", "08:30": "08:30", "00:00": "00:00", "23:59": "23:59",
	} {
		got, rerr := parseStartTime(SheetClasses, 3, "Giờ", in)
		require.Nil(t, rerr, "input %q", in)
		require.Equal(t, want, got, "input %q", in)
	}
	for _, in := range []string{"18:0", "25:00", "18:60", "6 giờ", "", "18h00"} {
		_, rerr := parseStartTime(SheetClasses, 3, "Giờ", in)
		require.NotNil(t, rerr, "input %q should be rejected", in)
	}
}

func TestParsePhone(t *testing.T) {
	t.Parallel()
	// Both accepted spellings collapse to the E.164 storage form, so a roster
	// written either way resolves to the same teacher and the same contact.
	for _, in := range []string{"0912345678", "+84912345678", "091 234 5678", "091.234.5678", "0912-345-678"} {
		got, rerr := parsePhone(SheetClasses, 3, "SĐT", in)
		require.Nil(t, rerr, "input %q", in)
		require.Equal(t, "+84912345678", got, "input %q", in)
	}
	// A blank cell is not an error here — on both sheets it means "the owner".
	got, rerr := parsePhone(SheetClasses, 3, "SĐT", "  ")
	require.Nil(t, rerr)
	require.Empty(t, got)
	// NormalizePhone alone would pass these through untouched; the pattern
	// check is what stops a malformed number reaching the database.
	for _, in := range []string{"84912345678", "+840912345678", "0112345678", "091234567", "abc"} {
		_, rerr := parsePhone(SheetClasses, 3, "SĐT", in)
		require.NotNil(t, rerr, "input %q should be rejected", in)
	}
}

func TestParseDuration(t *testing.T) {
	t.Parallel()
	got, rerr := parseDuration(SheetClasses, 3, "Thời lượng", "")
	require.Nil(t, rerr)
	require.Equal(t, int16(defaultDuraMin), got, "a blank cell means the 90-minute default")

	got, rerr = parseDuration(SheetClasses, 3, "Thời lượng", "120")
	require.Nil(t, rerr)
	require.Equal(t, int16(120), got)

	for _, in := range []string{"0", "-30", "90.5", "1441", "hai tiếng"} {
		_, rerr := parseDuration(SheetClasses, 3, "Thời lượng", in)
		require.NotNil(t, rerr, "input %q should be rejected", in)
	}
}

func TestCleanNameFoldsUnicodeForms(t *testing.T) {
	t.Parallel()
	// macOS Excel writes NFD, the web UI writes NFC. Postgres compares VARCHAR
	// bytewise, so without folding these are two different classes and every
	// natural-key lookup in the write path misses.
	nfc := "Toán 9A"  // "Toán 9A" precomposed
	nfd := "Toán 9A" // "Toa" + combining acute
	require.NotEqual(t, nfc, nfd, "the two forms must differ before folding")
	require.Equal(t, cleanName(nfc), cleanName(nfd))
	require.Equal(t, nfc, cleanName("  "+nfd+"  "), "trim then fold")
}

func TestCappedCountsRunesNotBytes(t *testing.T) {
	t.Parallel()
	// VARCHAR(n) counts characters, and Vietnamese is multi-byte: a byte-based
	// cap would reject a legal 40-character name.
	name := ""
	for range 40 {
		name += "ế"
	}
	_, rerr := capped(SheetStudents, 3, "Họ tên", name, MaxDisplayNote)
	require.Nil(t, rerr, "40 runes is under the 50-rune cap despite being 120 bytes")

	long := name + name // 80 runes
	_, rerr = capped(SheetStudents, 3, "Họ tên", long, MaxDisplayNote)
	require.NotNil(t, rerr)
	require.Equal(t, CodeTooLong, rerr.Code)
}
