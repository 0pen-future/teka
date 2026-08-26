package audit

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// fakeLister records the spec the service builds and returns scripted rows.
type fakeLister struct {
	spec ListSpec
	rows []Row
	err  error
}

func (f *fakeLister) List(_ context.Context, spec ListSpec) ([]Row, error) {
	f.spec = spec
	return f.rows, f.err
}

func ownerScope() authctx.Scope {
	return authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: true}
}

// listRows builds n rows with strictly descending occurred_at, newest first,
// matching the order the repository returns.
func listRows(n int) []Row {
	base := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	rows := make([]Row, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, Row{Log: Log{
			ID:         id.New(),
			OccurredAt: base.Add(-time.Duration(i) * time.Second),
			Action:     "class.create",
		}})
	}
	return rows
}

func TestListRequiresOwner(t *testing.T) {
	svc := NewService(&fakeLister{})
	sc := ownerScope()
	sc.IsOwner = false
	_, _, err := svc.List(context.Background(), sc, ListQuery{})
	if apperror.From(err).Code != apperror.CodeForbidden {
		t.Fatalf("member list error = %v, want FORBIDDEN", err)
	}
}

// TestListClampsLimit proves the default and the cap, and that the store is
// asked for one extra row so a full page can prove or disprove a next page.
func TestListClampsLimit(t *testing.T) {
	cases := []struct {
		in, wantFetch int
	}{
		{0, 51}, {-5, 51}, {999, 101}, {70, 71}, {1, 2},
	}
	for _, tc := range cases {
		store := &fakeLister{}
		svc := NewService(store)
		if _, _, err := svc.List(context.Background(), ownerScope(), ListQuery{Limit: tc.in}); err != nil {
			t.Fatalf("limit %d: %v", tc.in, err)
		}
		if store.spec.Limit != tc.wantFetch {
			t.Errorf("limit %d: fetch = %d, want %d", tc.in, store.spec.Limit, tc.wantFetch)
		}
	}
}

// TestListNextCursorRoundtrip proves a full page yields a cursor pointing at
// its last visible row, and that feeding the cursor back reaches the store
// as the exact keyset position.
func TestListNextCursorRoundtrip(t *testing.T) {
	store := &fakeLister{rows: listRows(51)}
	svc := NewService(store)

	rows, next, err := svc.List(context.Background(), ownerScope(), ListQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 50 {
		t.Fatalf("rows = %d, want 50 (the +1 probe row must not leak)", len(rows))
	}
	if next == "" {
		t.Fatal("next cursor empty on a full page")
	}

	last := rows[len(rows)-1]
	store2 := &fakeLister{}
	svc2 := NewService(store2)
	if _, _, err := svc2.List(context.Background(), ownerScope(), ListQuery{Cursor: next}); err != nil {
		t.Fatalf("cursor round-trip rejected: %v", err)
	}
	if !store2.spec.CursorAt.Equal(last.OccurredAt) || store2.spec.CursorID != last.ID {
		t.Errorf("cursor decoded to (%v, %v), want (%v, %v)",
			store2.spec.CursorAt, store2.spec.CursorID, last.OccurredAt, last.ID)
	}
}

func TestListNoCursorOnShortPage(t *testing.T) {
	store := &fakeLister{rows: listRows(20)}
	svc := NewService(store)
	rows, next, err := svc.List(context.Background(), ownerScope(), ListQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 20 || next != "" {
		t.Fatalf("rows = %d next = %q, want 20 rows and no cursor", len(rows), next)
	}
}

func TestListRejectsGarbageCursor(t *testing.T) {
	for _, cur := range []string{"not-base64!", "aGVsbG8", "MjAyNnwbm90LXV1aWQ"} {
		svc := NewService(&fakeLister{})
		_, _, err := svc.List(context.Background(), ownerScope(), ListQuery{Cursor: cur})
		if apperror.From(err).Code != apperror.CodeBadRequest {
			t.Errorf("cursor %q: error = %v, want BAD_REQUEST", cur, err)
		}
	}
}
