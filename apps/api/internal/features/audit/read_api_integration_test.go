//go:build integration

package audit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/audit"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/testutil"
)

// seedLog inserts one audit row directly; mutate the returned-by-value row
// through mut before insertion.
func seedLog(t *testing.T, db *gorm.DB, mut func(*audit.Log)) audit.Log {
	t.Helper()
	row := audit.Log{
		ID:         id.New(),
		OccurredAt: time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
		Action:     "class.create",
		Method:     http.MethodPost,
		Path:       "/api/v1/classes",
		EntityType: "class",
		StatusCode: http.StatusCreated,
	}
	if mut != nil {
		mut(&row)
	}
	require.NoError(t, db.Create(&row).Error)
	return row
}

func actionsOf(rows []audit.Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Action)
	}
	return out
}

// setMembership pins one membership stint's window to exact timestamps so
// the visibility assertions never race the fixtures' now() defaults.
func setMembership(t *testing.T, db *gorm.DB, teacherID, centerID uuid.UUID, joined time.Time, left *time.Time) {
	t.Helper()
	res := db.Exec("UPDATE center_members SET joined_at = ?, left_at = ? WHERE teacher_id = ? AND center_id = ?",
		joined, left, teacherID, centerID)
	require.NoError(t, res.Error)
	require.EqualValues(t, 1, res.RowsAffected)
}

// TestListVisibilityAndMembership proves the two visibility legs together:
// center-stamped rows stay inside their center, and center-less auth rows
// surface only through the membership window in which they occurred — they
// do not follow the teacher to a new center, and they stay in the old
// center's trail after the teacher leaves. A failed login (no actor)
// surfaces to nobody, and a plain member is refused outright.
func TestListVisibilityAndMembership(t *testing.T) {
	db := testutil.StartPostgres(t)
	ctx := context.Background()
	svc := audit.NewService(audit.NewRepository(db))

	joinA := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	leaveA := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	joinB := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	ownerA, teacherA := testutil.Teacher(t, db, testutil.WithFullName("Owner A"))
	ownerB, teacherB := testutil.Teacher(t, db, testutil.WithFullName("Owner B"))
	memberA, _ := testutil.Teacher(t, db, testutil.WithFullName("Member A"))
	testutil.JoinCenter(t, db, memberA.ID, teacherA.CenterID)
	setMembership(t, db, memberA.ID, teacherA.CenterID, joinA, nil)
	setMembership(t, db, ownerB.ID, teacherB.CenterID, joinA, nil)

	seedLog(t, db, func(l *audit.Log) {
		l.CenterID = &teacherA.CenterID
		l.ActorUserID = &ownerA.ID
	})
	seedLog(t, db, func(l *audit.Log) {
		l.CenterID = &teacherB.CenterID
		l.ActorUserID = &ownerB.ID
		l.Action = "class.update"
	})
	// Inside member A's stint at center A.
	seedLog(t, db, func(l *audit.Log) {
		l.Action = "auth.login"
		l.EntityType = "session"
		l.ActorUserID = &memberA.ID
		l.OccurredAt = time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	})
	// Before member A joined center A: no center may surface it.
	seedLog(t, db, func(l *audit.Log) {
		l.Action = "auth.logout"
		l.EntityType = "session"
		l.ActorUserID = &memberA.ID
		l.OccurredAt = time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	})
	seedLog(t, db, func(l *audit.Log) {
		l.Action = "auth.login"
		l.EntityType = "session"
		l.ActorUserID = &ownerB.ID
	})
	seedLog(t, db, func(l *audit.Log) {
		l.Action = "auth.login_fail"
		l.EntityType = "session"
		l.StatusCode = http.StatusUnauthorized
		l.Metadata = audit.Metadata{"phone_masked": "090***123"}
	})

	memberScope := testutil.ScopeFor(t, db, memberA.ID)
	require.False(t, memberScope.IsOwner)
	require.Equal(t, teacherA.CenterID, memberScope.CenterID)
	_, _, err := svc.List(ctx, memberScope, audit.ListQuery{})
	require.Error(t, err, "a plain member must not read the trail")

	assertCenterA := func() {
		t.Helper()
		scopeA := testutil.ScopeFor(t, db, ownerA.ID)
		require.True(t, scopeA.IsOwner)
		rowsA, next, err := svc.List(ctx, scopeA, audit.ListQuery{})
		require.NoError(t, err)
		require.Empty(t, next)
		require.Len(t, rowsA, 2, "owner A must see exactly their mutation and their member's in-window auth row, got %v", actionsOf(rowsA))
		for _, r := range rowsA {
			switch r.Action {
			case "class.create":
				require.Equal(t, "Owner A", r.ActorName, "actor name must resolve via teachers join")
			case "auth.login":
				require.Equal(t, memberA.ID, *r.ActorUserID, "only the member's in-window auth row is visible in center A")
				require.Equal(t, "Member A", r.ActorName)
			default:
				t.Fatalf("unexpected action %q visible to owner A", r.Action)
			}
		}
	}
	assertCenterB := func() {
		t.Helper()
		rowsB, _, err := svc.List(ctx, testutil.ScopeFor(t, db, ownerB.ID), audit.ListQuery{})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"class.update", "auth.login"}, actionsOf(rowsB))
		for _, r := range rowsB {
			require.Equal(t, ownerB.ID, *r.ActorUserID, "member A's history must not follow them into center B")
		}
	}
	assertCenterA()
	assertCenterB()

	// Member A leaves center A and joins center B: the in-window login must
	// stay in A's trail and must not appear in B's.
	left := leaveA
	setMembership(t, db, memberA.ID, teacherA.CenterID, joinA, &left)
	testutil.JoinCenter(t, db, memberA.ID, teacherB.CenterID)
	setMembership(t, db, memberA.ID, teacherB.CenterID, joinB, nil)

	assertCenterA()
	assertCenterB()
}

// TestListFilters proves each query filter against real SQL: actor, time
// window, and action prefix — including that LIKE metacharacters in the
// prefix are literal, so "class_" cannot match "classX".
func TestListFilters(t *testing.T) {
	db := testutil.StartPostgres(t)
	ctx := context.Background()
	svc := audit.NewService(audit.NewRepository(db))

	owner, teacher := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, member.ID, teacher.CenterID)
	scope := testutil.ScopeFor(t, db, owner.ID)

	base := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	seed := func(action string, actorID uuid.UUID, at time.Time) {
		seedLog(t, db, func(l *audit.Log) {
			l.CenterID = &teacher.CenterID
			l.ActorUserID = &actorID
			l.Action = action
			l.OccurredAt = at
		})
	}
	seed("class.create", owner.ID, base)
	seed("class.update", member.ID, base.Add(1*time.Hour))
	seed("collection.create", owner.ID, base.Add(2*time.Hour))
	seed("class_x.create", owner.ID, base.Add(3*time.Hour))
	seed("classZx.create", owner.ID, base.Add(4*time.Hour))

	rows, _, err := svc.List(ctx, scope, audit.ListQuery{ActorID: member.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"class.update"}, actionsOf(rows))

	rows, _, err = svc.List(ctx, scope, audit.ListQuery{Action: "class."})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"class.create", "class.update"}, actionsOf(rows))

	rows, _, err = svc.List(ctx, scope, audit.ListQuery{Action: "class_"})
	require.NoError(t, err)
	require.Equal(t, []string{"class_x.create"}, actionsOf(rows),
		"underscore in the prefix must be literal, not a LIKE wildcard")

	rows, _, err = svc.List(ctx, scope, audit.ListQuery{
		From: base.Add(30 * time.Minute),
		To:   base.Add(90 * time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, []string{"class.update"}, actionsOf(rows))
}

// TestListKeysetPaginationStable walks 120 rows squeezed onto 3 shared
// timestamps through 50/50/20 pages, proving the (occurred_at, id) keyset
// neither skips nor repeats rows even when the timestamp alone cannot order
// them.
func TestListKeysetPaginationStable(t *testing.T) {
	db := testutil.StartPostgres(t)
	ctx := context.Background()
	svc := audit.NewService(audit.NewRepository(db))

	owner, teacher := testutil.Teacher(t, db)
	scope := testutil.ScopeFor(t, db, owner.ID)

	base := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	const total = 120
	inserted := make(map[uuid.UUID]bool, total)
	for i := 0; i < total; i++ {
		at := base.Add(time.Duration(i%3) * time.Minute) // 40 rows per timestamp
		row := seedLog(t, db, func(l *audit.Log) {
			l.CenterID = &teacher.CenterID
			l.ActorUserID = &owner.ID
			l.OccurredAt = at
		})
		inserted[row.ID] = true
	}

	seen := make(map[uuid.UUID]bool, total)
	var pageSizes []int
	cursor := ""
	var prev *audit.Row
	for page := 0; ; page++ {
		require.Less(t, page, 10, "pagination must terminate")
		rows, next, err := svc.List(ctx, scope, audit.ListQuery{Limit: 50, Cursor: cursor})
		require.NoError(t, err)
		pageSizes = append(pageSizes, len(rows))
		for i := range rows {
			r := &rows[i]
			require.False(t, seen[r.ID], "row %s returned twice", r.ID)
			require.True(t, inserted[r.ID], "row %s was never inserted", r.ID)
			seen[r.ID] = true
			if prev != nil {
				afterPrev := r.OccurredAt.Before(prev.OccurredAt) ||
					(r.OccurredAt.Equal(prev.OccurredAt) && r.ID.String() < prev.ID.String())
				require.True(t, afterPrev, "rows must be strictly (occurred_at, id) descending across pages")
			}
			prev = r
		}
		if next == "" {
			break
		}
		cursor = next
	}

	require.Equal(t, []int{50, 50, 20}, pageSizes)
	require.Len(t, seen, total, "no row may be skipped")
}

// TestReadEndpointHTTP drives the mounted route end to end: envelope shape
// and next_cursor for the owner, 401 without a resolved scope, 403 for a
// member, and 400 on a malformed filter.
func TestReadEndpointHTTP(t *testing.T) {
	db := testutil.StartPostgres(t)
	svc := audit.NewService(audit.NewRepository(db))

	owner, teacher := testutil.Teacher(t, db, testutil.WithFullName("Owner HTTP"))
	seedLog(t, db, func(l *audit.Log) {
		l.CenterID = &teacher.CenterID
		l.ActorUserID = &owner.ID
		l.Metadata = audit.Metadata{"member_id": "01a03d01-0000-7000-8000-000000000001"}
	})

	gin.SetMode(gin.TestMode)
	newRouter := func(scope *authctx.Scope) *gin.Engine {
		r := gin.New()
		v1 := r.Group("/api/v1")
		// Stand-ins for RequireAuth+ResolveScope: the handler consumes only
		// the resolved scope, so the stub either sets one or leaves the
		// context bare to exercise the 401 path.
		setScope := func(c *gin.Context) {
			if scope != nil {
				authctx.SetScope(c, *scope)
			}
		}
		audit.RegisterRoutes(v1, audit.NewHandler(svc), setScope, func(*gin.Context) {})
		return r
	}
	get := func(r *gin.Engine, target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		return w
	}

	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	w := get(newRouter(&ownerScope), "/api/v1/audit-logs?limit=50")
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Action    string            `json:"action"`
				ActorName string            `json:"actor_name"`
				Metadata  map[string]string `json:"metadata"`
			} `json:"items"`
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data.Items, 1)
	require.Equal(t, "class.create", body.Data.Items[0].Action)
	require.Equal(t, "Owner HTTP", body.Data.Items[0].ActorName)
	require.Equal(t, map[string]string{"member_id": "01a03d01-0000-7000-8000-000000000001"},
		body.Data.Items[0].Metadata, "stored metadata must reach the wire unchanged")
	require.Empty(t, body.Data.NextCursor)

	require.Equal(t, http.StatusUnauthorized, get(newRouter(nil), "/api/v1/audit-logs").Code)

	memberScope := ownerScope
	memberScope.IsOwner = false
	require.Equal(t, http.StatusForbidden, get(newRouter(&memberScope), "/api/v1/audit-logs").Code)

	require.Equal(t, http.StatusBadRequest,
		get(newRouter(&ownerScope), "/api/v1/audit-logs?actor_id=not-a-uuid").Code)
	require.Equal(t, http.StatusBadRequest,
		get(newRouter(&ownerScope), "/api/v1/audit-logs?from=yesterday").Code)
	require.Equal(t, http.StatusBadRequest,
		get(newRouter(&ownerScope), "/api/v1/audit-logs?cursor=%21garbage").Code)
}
