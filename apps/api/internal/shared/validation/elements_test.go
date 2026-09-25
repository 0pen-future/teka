package validation

import "testing"

type elementFixture struct {
	Name string   `json:"name" binding:"required,max=3"`
	Kind string   `json:"kind" binding:"required,oneof=a b"`
	Opts []string `json:"opts" binding:"omitempty,dive,max=2"`
}

func TestElementsKeysFailuresByIndex(t *testing.T) {
	if err := Elements([]elementFixture{{Name: "ok", Kind: "a"}}); err != nil {
		t.Fatalf("valid rows must pass, got %+v", err)
	}
	if err := Elements[elementFixture](nil); err != nil {
		t.Fatalf("empty body must pass, got %+v", err)
	}
	err := Elements([]elementFixture{{Name: "ok", Kind: "a"}, {Name: "", Kind: "c"}, {Name: "long", Kind: "b", Opts: []string{"ok", "long"}}})
	if err == nil || err.Status != 422 {
		t.Fatalf("want 422, got %+v", err)
	}
	// A failing slice element is keyed by the field, not by "opts[1]", so
	// clients can map it onto the row's input.
	want := map[string]string{
		"1.name": "is required",
		"1.kind": "must be one of: a, b",
		"2.name": "must be at most 3 characters",
		"2.opts": "must be at most 2 characters",
	}
	if len(err.Fields) != len(want) {
		t.Fatalf("fields %+v, want %+v", err.Fields, want)
	}
	for k, v := range want {
		if err.Fields[k] != v {
			t.Fatalf("field %s = %q, want %q", k, err.Fields[k], v)
		}
	}
}
