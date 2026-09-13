//go:build integration

package tasks_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/tasks"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/testutil"
)

// tasksEnv bundles a real, DB-backed tasks.Service plus the raw handle
// needed to seed fixtures directly and a SyncBus so event assertions do not
// race the test goroutine. Shared by every *_integration_test.go file in
// this package.
type tasksEnv struct {
	db  *gorm.DB
	bus *events.SyncBus
	svc *tasks.Service
}

func newTasksEnv(t *testing.T) *tasksEnv {
	t.Helper()
	db := testutil.StartPostgres(t)
	bus := events.NewSync()
	svc := tasks.NewService(db, database.NewTxManager(db), bus)
	return &tasksEnv{db: db, bus: bus, svc: svc}
}

// grantPerm gives teacherID an explicit member-level grant on centerID,
// mirroring how an owner opts a member into tasks.manage_board/
// tasks.view_all/members.list — permissions the default RBAC backfill
// deliberately withholds.
func grantPerm(t *testing.T, db *gorm.DB, teacherID, centerID uuid.UUID, key string) {
	t.Helper()
	require.NoError(t, db.Exec(`
		INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		VALUES (?, ?, ?, TRUE)
		ON CONFLICT (teacher_id, center_id, permission_key) DO UPDATE SET allowed = TRUE`,
		teacherID, centerID, key).Error)
}

// seedColumn inserts a task_columns row directly, bypassing the service.
func seedColumn(t *testing.T, db *gorm.DB, centerID uuid.UUID, name string, position int, isDone bool) uuid.UUID {
	t.Helper()
	colID := id.New()
	require.NoError(t, db.Exec(
		`INSERT INTO task_columns (id, center_id, name, position, is_done) VALUES (?, ?, ?, ?, ?)`,
		colID, centerID, name, position, isDone).Error)
	return colID
}

// seedTaskOpts customizes a fixture task row before insertion.
type seedTaskOpts struct {
	assignee  *uuid.UUID
	position  float64
	deletedAt bool
}

// seedTask inserts a tasks row directly, bypassing the service.
func seedTask(t *testing.T, db *gorm.DB, centerID, columnID, createdBy uuid.UUID, title string, opts seedTaskOpts) uuid.UUID {
	t.Helper()
	taskID := id.New()
	deletedExpr := "NULL"
	if opts.deletedAt {
		deletedExpr = "now()"
	}
	require.NoError(t, db.Exec(
		`INSERT INTO tasks (id, center_id, column_id, created_by, assignee_id, title, priority, position, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'none', ?, `+deletedExpr+`)`,
		taskID, centerID, columnID, createdBy, opts.assignee, title, opts.position).Error)
	return taskID
}

// TestBoardOrdersColumnsByPosition asserts GET board's columns come back in
// Position order regardless of insertion order.
func TestBoardOrdersColumnsByPosition(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)

	third := seedColumn(t, e.db, scope.CenterID, "third", 2, false)
	first := seedColumn(t, e.db, scope.CenterID, "first", 0, false)
	second := seedColumn(t, e.db, scope.CenterID, "second", 1, false)

	resp, err := e.svc.Board(context.Background(), scope)
	require.NoError(t, err)
	require.Len(t, resp.Columns, 3)
	require.Equal(t, first, resp.Columns[0].ID)
	require.Equal(t, second, resp.Columns[1].ID)
	require.Equal(t, third, resp.Columns[2].ID)
}

// TestBoardCapsAt50TasksAndReportsHasMore asserts a 51-task column is capped
// server-side, with the excess flagged via has_more rather than silently
// dropped.
func TestBoardCapsAt50TasksAndReportsHasMore(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)

	for i := 0; i < 51; i++ {
		seedTask(t, e.db, scope.CenterID, col, owner.ID, "t", seedTaskOpts{position: float64(i)})
	}

	resp, err := e.svc.Board(context.Background(), scope)
	require.NoError(t, err)
	require.Len(t, resp.Columns, 1)
	require.Len(t, resp.Columns[0].Tasks, 50)
	require.True(t, resp.Columns[0].HasMore)
}

// TestBoardOrdersTasksByPositionThenCreatedAt asserts tasks within a column
// come back in the same order the repository's ORDER BY (position,
// created_at) declares.
func TestBoardOrdersTasksByPositionThenCreatedAt(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)

	last := seedTask(t, e.db, scope.CenterID, col, owner.ID, "last", seedTaskOpts{position: 2})
	first := seedTask(t, e.db, scope.CenterID, col, owner.ID, "first", seedTaskOpts{position: 0})
	middle := seedTask(t, e.db, scope.CenterID, col, owner.ID, "middle", seedTaskOpts{position: 1})

	resp, err := e.svc.Board(context.Background(), scope)
	require.NoError(t, err)
	require.Len(t, resp.Columns[0].Tasks, 3)
	require.Equal(t, first, resp.Columns[0].Tasks[0].ID)
	require.Equal(t, middle, resp.Columns[0].Tasks[1].ID)
	require.Equal(t, last, resp.Columns[0].Tasks[2].ID)
}

// TestBoardExcludesSoftDeletedTasks asserts a soft-deleted task never
// surfaces on the board, capped-column bookkeeping aside.
func TestBoardExcludesSoftDeletedTasks(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "gone", seedTaskOpts{deletedAt: true})

	resp, err := e.svc.Board(context.Background(), scope)
	require.NoError(t, err)
	require.Empty(t, resp.Columns[0].Tasks)
}
