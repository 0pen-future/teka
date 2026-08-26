//go:build integration

package audit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/testutil"
)

// TestListQueryPlanUsesIndexes EXPLAINs the exact SQL Repository.List emits
// (via listSQL) against analyzed data, proving both visibility legs are
// index-served with their LIMITs pushed down: the center leg walks
// idx_audit_logs_center_time in order, the membership leg walks
// idx_audit_logs_actor per member, and the base table is never
// sequentially scanned — so a page costs O(limit), not O(trail). The full
// plan lands in the test log for the phase report.
func TestListQueryPlanUsesIndexes(t *testing.T) {
	db := testutil.StartPostgres(t)
	ctx := context.Background()
	repo := NewRepository(db)

	owner, teacher := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, member.ID, teacher.CenterID)

	base := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	rows := make([]Log, 0, 2400)
	for i := 0; i < 2000; i++ {
		rows = append(rows, Log{
			ID:          id.New(),
			OccurredAt:  base.Add(-time.Duration(i) * time.Minute),
			CenterID:    &teacher.CenterID,
			ActorUserID: &owner.ID,
			Action:      "class.create",
			Method:      "POST",
			Path:        "/api/v1/classes",
			EntityType:  "class",
			StatusCode:  201,
		})
	}
	for i := 0; i < 400; i++ {
		rows = append(rows, Log{
			ID:          id.New(),
			OccurredAt:  base.Add(-time.Duration(i) * time.Minute),
			ActorUserID: &member.ID,
			Action:      "auth.login",
			Method:      "POST",
			Path:        "/api/v1/auth/login",
			EntityType:  "session",
			StatusCode:  200,
		})
	}
	require.NoError(t, repo.InsertBatch(ctx, rows))
	require.NoError(t, db.Exec("ANALYZE audit_logs, center_members, teachers").Error)

	// The second-page shape is the one that matters: cursor predicate plus
	// the probe limit, exactly as the service builds it.
	query, args := listSQL(ListSpec{
		CenterID: teacher.CenterID,
		CursorAt: base.Add(-50 * time.Minute),
		CursorID: id.New(),
		Limit:    51,
	})
	var lines []string
	require.NoError(t, db.Raw("EXPLAIN "+query, args...).Scan(&lines).Error)
	plan := strings.Join(lines, "\n")
	t.Logf("query plan:\n%s", plan)

	require.Contains(t, plan, "Index Scan using idx_audit_logs_center_time",
		"the center leg must be served in order by the center+time index")
	require.Contains(t, plan, "idx_audit_logs_actor",
		"the membership leg must be served per member by the actor index")
	require.NotContains(t, plan, "Seq Scan on audit_logs",
		"no page may scan the whole trail")
}
