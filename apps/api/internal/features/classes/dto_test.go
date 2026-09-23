package classes

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The response derives phase from the stored dates and never serialises
// tags as null, so the web client can render both without fallbacks.
func TestFromModelCarriesPhaseAndTags(t *testing.T) {
	class := &Class{
		Name:      "Toán 8",
		Code:      "TOAN8",
		StartDate: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Status:    StatusActive,
	}
	resp := FromModel(class)
	if resp.Code != "TOAN8" || resp.Phase != PhaseOf(class, today()) {
		t.Fatalf("code and phase must map, got %+v", resp)
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"tags":[]`, `"recruiting":false`, `"note":null`, `"phase":"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("response must contain %s, got %s", want, body)
		}
	}
}
