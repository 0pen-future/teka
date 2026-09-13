package tasks

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/pkg/kanban"
)

// taskRepository implements kanban.TaskRepository against tasks.
type taskRepository struct {
	db *gorm.DB
}

// newTaskRepository builds the GORM-backed kanban.TaskRepository.
func newTaskRepository(db *gorm.DB) *taskRepository {
	return &taskRepository{db: db}
}

var _ kanban.TaskRepository = (*taskRepository)(nil)

// ListBoard returns tenant's live (non soft-deleted) tasks narrowed by vis,
// ordered by column then position then creation time — the order
// Service.Board's caller renders the board in. When limit > 0, at most limit
// tasks are returned per column (same order), via a window function, so a
// caller asking only for a display page never pays for a full tenant scan
// (see pkg/kanban/service.go's Board doc comment for why 0 means unlimited).
func (r *taskRepository) ListBoard(ctx context.Context, tenant kanban.TenantID, vis kanban.Visibility, limit int) ([]kanban.Task, error) {
	db := database.FromContext(ctx, r.db)
	participant := uuid.UUID(vis.Participant)

	var rows []taskModel
	if limit <= 0 {
		q := db.Where("center_id = ? AND deleted_at IS NULL", uuid.UUID(tenant))
		if !vis.All {
			q = q.Where("created_by = ? OR assignee_id = ?", participant, participant)
		}
		if err := q.Order("column_id, position, created_at").Find(&rows).Error; err != nil {
			return nil, err
		}
	} else {
		const query = `
			SELECT id, center_id, column_id, created_by, assignee_id, title,
			       description, priority, due_on, position, completed_at,
			       created_at, updated_at, deleted_at
			FROM (
				SELECT *, row_number() OVER (
					PARTITION BY column_id ORDER BY position, created_at
				) AS rn
				FROM tasks
				WHERE center_id = ? AND deleted_at IS NULL
					AND (? OR created_by = ? OR assignee_id = ?)
			) ranked
			WHERE rn <= ?
			ORDER BY column_id, position, created_at`
		if err := db.Raw(query, uuid.UUID(tenant), vis.All, participant, participant, limit).
			Scan(&rows).Error; err != nil {
			return nil, err
		}
	}

	tasks := make([]kanban.Task, len(rows))
	for i, row := range rows {
		tasks[i] = taskToCore(row)
	}
	return tasks, nil
}

// MinPositionInColumn returns the lowest Position among col's live tasks,
// using idx_tasks_board (center_id, column_id, position). It replaces
// loading and scanning every visible task in the tenant just to compute
// where a new/moved task lands (pkg/kanban/service.go's
// topPositionInColumn).
func (r *taskRepository) MinPositionInColumn(ctx context.Context, tenant kanban.TenantID, col kanban.ColumnID) (float64, bool, error) {
	var minPos *float64
	err := database.FromContext(ctx, r.db).
		Model(&taskModel{}).
		Where("center_id = ? AND column_id = ? AND deleted_at IS NULL", uuid.UUID(tenant), uuid.UUID(col)).
		Select("MIN(position)").
		Scan(&minPos).Error
	if err != nil {
		return 0, false, err
	}
	if minPos == nil {
		return 0, false, nil
	}
	return *minPos, true, nil
}

// Get returns the live task identified by id within tenant, or
// kanban.ErrTaskNotFound if it does not exist, is soft-deleted, or belongs
// to another tenant.
func (r *taskRepository) Get(ctx context.Context, tenant kanban.TenantID, id kanban.TaskID) (kanban.Task, error) {
	var row taskModel
	err := database.FromContext(ctx, r.db).
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", uuid.UUID(id), uuid.UUID(tenant)).
		First(&row).Error
	if err != nil {
		if errIsRecordNotFound(err) {
			return kanban.Task{}, kanban.ErrTaskNotFound
		}
		return kanban.Task{}, err
	}
	return taskToCore(row), nil
}

// Create inserts task, stamping CenterID from tenant explicitly rather than
// trusting task.TenantID to already carry it — the same defense every other
// method in this file applies by filtering on tenant, applied here at write
// time instead of read time.
func (r *taskRepository) Create(ctx context.Context, tenant kanban.TenantID, task kanban.Task) (kanban.Task, error) {
	row := taskFromCore(task)
	row.CenterID = uuid.UUID(tenant)
	if err := translateDBError(database.FromContext(ctx, r.db).Create(&row).Error); err != nil {
		return kanban.Task{}, err
	}
	return taskToCore(row), nil
}

// Update persists task's full editable state (column, title, description,
// priority, due date, assignee, position, completion) over the live row
// matching its id and tenant — both Service.UpdateTask and Service.MoveTask
// route through this one method, each having already merged its own change
// into a task loaded via Get.
func (r *taskRepository) Update(ctx context.Context, tenant kanban.TenantID, task kanban.Task) (kanban.Task, error) {
	row := taskFromCore(task)
	res := database.FromContext(ctx, r.db).
		Model(&taskModel{}).
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", uuid.UUID(task.ID), uuid.UUID(tenant)).
		Updates(map[string]any{
			"column_id":    row.ColumnID,
			"title":        row.Title,
			"description":  row.Description,
			"priority":     row.Priority,
			"due_on":       row.DueOn,
			"assignee_id":  row.AssigneeID,
			"position":     row.Position,
			"completed_at": row.CompletedAt,
			"updated_at":   gorm.Expr("now()"),
		})
	if res.Error != nil {
		return kanban.Task{}, translateDBError(res.Error)
	}
	if res.RowsAffected == 0 {
		return kanban.Task{}, kanban.ErrTaskNotFound
	}
	return r.Get(ctx, tenant, task.ID)
}

// SoftDelete stamps deleted_at on the live task identified by id within
// tenant.
func (r *taskRepository) SoftDelete(ctx context.Context, tenant kanban.TenantID, id kanban.TaskID) error {
	res := database.FromContext(ctx, r.db).
		Model(&taskModel{}).
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", uuid.UUID(id), uuid.UUID(tenant)).
		Update("deleted_at", gorm.Expr("now()"))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return kanban.ErrTaskNotFound
	}
	return nil
}

// CountInColumn counts every task in col, including soft-deleted ones: the
// database still enforces fk_tasks_column_center against them, so
// DeleteColumn must know about them too (see pkg/kanban/ports.go's doc
// comment on TaskRepository.CountInColumn).
func (r *taskRepository) CountInColumn(ctx context.Context, tenant kanban.TenantID, col kanban.ColumnID) (int, error) {
	var count int64
	err := database.FromContext(ctx, r.db).
		Model(&taskModel{}).
		Where("center_id = ? AND column_id = ?", uuid.UUID(tenant), uuid.UUID(col)).
		Count(&count).Error
	return int(count), err
}

// MoveAllToColumn re-points every task (soft-deleted included, same
// rationale as CountInColumn) from column from to column to, setting
// CompletedAt uniformly to completedAt (nil clears it, non-nil stamps it —
// Service computes this once from the destination column's IsDone before
// calling in).
func (r *taskRepository) MoveAllToColumn(ctx context.Context, tenant kanban.TenantID, from, to kanban.ColumnID, completedAt *time.Time) (int, error) {
	res := database.FromContext(ctx, r.db).
		Model(&taskModel{}).
		Where("center_id = ? AND column_id = ?", uuid.UUID(tenant), uuid.UUID(from)).
		Updates(map[string]any{
			"column_id":    uuid.UUID(to),
			"completed_at": completedAt,
			"updated_at":   gorm.Expr("now()"),
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

// UnassignBy clears assignee_id on every live task in tenant assigned to
// actor. Soft-deleted tasks are left untouched: they no longer surface
// anywhere a caller can read, so there is nothing for a departure handover
// to fix on them.
func (r *taskRepository) UnassignBy(ctx context.Context, tenant kanban.TenantID, actor kanban.ActorID) (int, error) {
	var noAssignee *uuid.UUID
	res := database.FromContext(ctx, r.db).
		Model(&taskModel{}).
		Where("center_id = ? AND assignee_id = ? AND deleted_at IS NULL", uuid.UUID(tenant), uuid.UUID(actor)).
		Updates(map[string]any{"assignee_id": noAssignee, "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

// ReassignCreator re-points created_by from from to to on every live task in
// tenant from created — same soft-delete rationale as UnassignBy.
func (r *taskRepository) ReassignCreator(ctx context.Context, tenant kanban.TenantID, from, to kanban.ActorID) (int, error) {
	res := database.FromContext(ctx, r.db).
		Model(&taskModel{}).
		Where("center_id = ? AND created_by = ? AND deleted_at IS NULL", uuid.UUID(tenant), uuid.UUID(from)).
		Updates(map[string]any{"created_by": uuid.UUID(to), "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}
