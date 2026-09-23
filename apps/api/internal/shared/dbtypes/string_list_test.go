package dbtypes

import (
	"testing"
)

func TestStringListValueWritesEmptyArrayForNil(t *testing.T) {
	var l StringList
	v, err := l.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got := string(v.([]byte)); got != "[]" {
		t.Fatalf("nil list must serialise as [] to satisfy NOT NULL DEFAULT '[]'; got %q", got)
	}

	v, err = StringList{"Toán", "Khối 9"}.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got := string(v.([]byte)); got != `["Toán","Khối 9"]` {
		t.Fatalf("unexpected JSON %q", got)
	}
}

func TestStringListScanAcceptsDriverForms(t *testing.T) {
	var fromBytes StringList
	if err := fromBytes.Scan([]byte(`["a","b"]`)); err != nil {
		t.Fatalf("Scan []byte: %v", err)
	}
	if len(fromBytes) != 2 || fromBytes[0] != "a" || fromBytes[1] != "b" {
		t.Fatalf("Scan []byte = %v", fromBytes)
	}

	var fromString StringList
	if err := fromString.Scan(`["x"]`); err != nil {
		t.Fatalf("Scan string: %v", err)
	}
	if len(fromString) != 1 || fromString[0] != "x" {
		t.Fatalf("Scan string = %v", fromString)
	}

	fromNil := StringList{"stale"}
	if err := fromNil.Scan(nil); err != nil {
		t.Fatalf("Scan nil: %v", err)
	}
	if fromNil != nil {
		t.Fatalf("Scan nil must reset the list, got %v", fromNil)
	}

	var bad StringList
	if err := bad.Scan(42); err == nil {
		t.Fatal("Scan int must fail")
	}
}
