package tasks

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func init() { gin.SetMode(gin.TestMode) }

// TestBoardQueryBindingRejectsOutOfContractValues asserts ShouldBindQuery
// enforces BoardQuery's oneof/uuid/datetime tags — the 422 boundary
// handler.board relies on before any filter ever reaches the service.
func TestBoardQueryBindingRejectsOutOfContractValues(t *testing.T) {
	cases := map[string]string{
		"filter outside the enum": "filter=bogus",
		"assignee not a uuid":     "assignee=abc",
		"today not YYYY-MM-DD":    "today=17-09-2026",
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/tasks/board?"+query, nil)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req

			var q BoardQuery
			if err := c.ShouldBindQuery(&q); err == nil {
				t.Fatalf("expected ShouldBindQuery to reject query %q", query)
			}
		})
	}
}

// TestBoardQueryBindingAcceptsEveryValidCombination asserts the reverse: the
// documented values, including the all-empty default, bind cleanly.
func TestBoardQueryBindingAcceptsEveryValidCombination(t *testing.T) {
	valid := []string{
		"",
		"filter=all",
		"filter=mine",
		"filter=overdue",
		"filter=today",
		"filter=unassigned",
		"assignee=" + uuid.NewString(),
		"today=2026-09-17",
	}
	for _, query := range valid {
		t.Run(query, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/tasks/board?"+query, nil)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req

			var q BoardQuery
			if err := c.ShouldBindQuery(&q); err != nil {
				t.Fatalf("expected query %q to bind cleanly, got %v", query, err)
			}
		})
	}
}
