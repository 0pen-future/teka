package students

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The student field list is closed by requirement, not by oversight. PRD R1
// limits student data to what fee calculation needs (name, owning contact,
// and the attendance-sheet disambiguator), and Nghị định 13/2023 makes every
// extra child-PII field a liability: "Bất kỳ đề xuất thêm trường nào cũng
// phải kèm câu trả lời 'trường này phục vụ tính tiền như thế nào' — nếu
// không trả lời được thì không thêm." If you are here because this test went
// red after adding a field, that is the process working: answer the PRD's
// question first, update the PRD, and only then extend this set.
var closedFieldList = []string{"contact_id", "display_note", "full_name"}

func jsonFields(t *testing.T, typ reflect.Type) []string {
	t.Helper()
	var fields []string
	for i := range typ.NumField() {
		tag := typ.Field(i).Tag.Get("json")
		name := strings.SplitN(tag, ",", 2)[0]
		if name == "" || name == "-" {
			t.Fatalf("field %s must carry a json tag", typ.Field(i).Name)
		}
		fields = append(fields, name)
	}
	sort.Strings(fields)
	return fields
}

func TestWritableDTOsExposeOnlyTheClosedFieldList(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"CreateRequest": reflect.TypeOf(CreateRequest{}),
		"UpdateRequest": reflect.TypeOf(UpdateRequest{}),
	} {
		if got := jsonFields(t, typ); !reflect.DeepEqual(got, closedFieldList) {
			t.Fatalf("%s must expose exactly %v, got %v — see the comment above closedFieldList", name, closedFieldList, got)
		}
	}
}
