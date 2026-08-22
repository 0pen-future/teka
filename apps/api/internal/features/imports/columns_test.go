package imports

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLengthCapsMatchDTOs pins this package's caps to the binding tags on the
// four CreateRequest DTOs. The import calls those services directly, below the
// gin binding layer that normally enforces the tags, so a cap that drifts here
// turns an operator-fixable row error into a mid-transaction 22001 with no
// line number — the exact failure this package exists to prevent.
func TestLengthCapsMatchDTOs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file   string
		strukt string
		field  string
		want   int
	}{
		{"../classes/dto.go", "CreateClassRequest", "Name", MaxClassName},
		{"../students/dto.go", "CreateRequest", "FullName", MaxFullName},
		{"../students/dto.go", "CreateRequest", "DisplayNote", MaxDisplayNote},
		{"../contacts/dto.go", "CreateRequest", "FullName", MaxFullName},
	}
	for _, c := range cases {
		t.Run(c.strukt+"."+c.field, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, c.want, dtoMaxTag(t, c.file, c.strukt, c.field),
				"%s.%s binding cap drifted from this package's cap", c.strukt, c.field)
		})
	}
}

// dtoMaxTag reads the max= binding tag off one named field of one named struct.
//
// It parses the file rather than matching a regex over the whole text. Every
// dto.go declares both a Create and an Update request with the same field
// names, so an unanchored regex silently falls through to the other struct: a
// removed cap, a renamed field, or a new field earlier in the file would all
// leave the test green while the pin was broken.
func dtoMaxTag(t *testing.T, file, structName, fieldName string) int {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)

	var tag string
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != structName {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, fld := range st.Fields.List {
			for _, name := range fld.Names {
				if name.Name == fieldName && fld.Tag != nil {
					tag = fld.Tag.Value
				}
			}
		}
		return false
	})
	require.NotEmpty(t, tag, "no field %s.%s with a struct tag in %s", structName, fieldName, file)

	m := regexp.MustCompile(`binding:"[^"]*\bmax=(\d+)`).FindStringSubmatch(tag)
	require.NotNil(t, m, "field %s.%s has no max= binding tag: %s", structName, fieldName, tag)
	n, err := strconv.Atoi(m[1])
	require.NoError(t, err)
	return n
}

// TestWeekdayVocabularyIsComplete guards the CN = 0 convention: class_schedules
// stores time.Weekday, so Sunday is 0 and never 8.
func TestWeekdayVocabularyIsComplete(t *testing.T) {
	t.Parallel()
	require.Len(t, weekdayFromSheet, 7)
	seen := map[int16]bool{}
	for _, v := range weekdayFromSheet {
		require.GreaterOrEqual(t, v, int16(0))
		require.LessOrEqual(t, v, int16(6))
		seen[v] = true
	}
	require.Len(t, seen, 7, "every weekday 0..6 must be reachable")
	require.Equal(t, int16(0), weekdayFromSheet["CN"])
}

// TestExampleRowsMatchHeaderWidth keeps the template's example row aligned
// with its header; a short example would shift the operator's mental mapping
// of which column is which.
func TestExampleRowsMatchHeaderWidth(t *testing.T) {
	t.Parallel()
	require.Len(t, exampleClassRow, len(classHeaders))
	require.Len(t, exampleStudentRow, len(studentHeaders))
}

// TestHeadersAreGolden makes renaming a column a deliberate act. Operators
// download the template once and fill it in for weeks; a rename silently
// invalidates every copy already in circulation, and a test that reads the
// same slice it verifies would never notice.
func TestHeadersAreGolden(t *testing.T) {
	t.Parallel()
	require.Equal(t, []string{
		"Tên lớp",
		"SĐT giáo viên",
		"Ngày khai giảng (dd/mm/yyyy)",
		"Đơn giá/buổi (đồng)",
		"Thứ (2-7 hoặc CN)",
		"Giờ bắt đầu (HH:MM)",
		"Thời lượng (phút)",
		"Ngày kết thúc (dd/mm/yyyy)",
	}, classHeaders)
	require.Equal(t, []string{
		"Họ tên học sinh",
		"Họ tên phụ huynh",
		"SĐT phụ huynh",
		"Tên lớp",
		"SĐT giáo viên",
		"Ngày nhập học (dd/mm/yyyy)",
		"Ghi chú phân biệt",
	}, studentHeaders)
}
