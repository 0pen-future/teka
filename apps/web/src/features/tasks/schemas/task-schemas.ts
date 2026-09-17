import { z } from "zod";

import { plainTextLength } from "../lib/rich-text";

/** `tasks.Priority` — plain enum, no numeric weighting on the wire. */
export const TASK_PRIORITIES = ["none", "low", "medium", "high"] as const;
export const taskPrioritySchema = z.enum(TASK_PRIORITIES);
export type TaskPriority = z.infer<typeof taskPrioritySchema>;

/** `task_columns.ColumnResponse.color` — a tint id, not a raw color value. */
export const COLUMN_COLORS = ["none", "sky", "sun", "mint"] as const;
export const columnColorSchema = z.enum(COLUMN_COLORS);
export type ColumnColor = z.infer<typeof columnColorSchema>;

/** `task_columns.ColumnResponse`. */
export const taskColumnSchema = z.object({
  id: z.string(),
  name: z.string(),
  position: z.number(),
  is_done: z.boolean(),
  color: columnColorSchema,
  created_at: z.string(),
  updated_at: z.string(),
});
export type TaskColumn = z.infer<typeof taskColumnSchema>;

/**
 * `tasks.TaskResponse`. Nullable fields are explicit `null` on the wire (no
 * Go `omitempty` for them), so `.nullable()` — not `.optional()` — mirrors
 * the contract; `position` is a float64 (smaller = higher in column).
 */
export const taskSchema = z.object({
  id: z.string(),
  column_id: z.string(),
  title: z.string(),
  description: z.string(),
  priority: taskPrioritySchema,
  due_on: z.string().nullable(),
  assignee_id: z.string().nullable(),
  created_by: z.string(),
  position: z.number(),
  completed_at: z.string().nullable(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Task = z.infer<typeof taskSchema>;

/** A board column carries its own tasks page (max 50, `has_more` flags the cut). */
export const boardColumnSchema = taskColumnSchema.extend({
  tasks: z.array(taskSchema),
  has_more: z.boolean(),
});
export type BoardColumn = z.infer<typeof boardColumnSchema>;

/** `GET /tasks/board`'s `filter` query param — server-side (see D3 in the plan), not a client-side selector. */
export const BOARD_FILTERS = ["all", "mine", "overdue", "today", "unassigned"] as const;
export const boardFilterSchema = z.enum(BOARD_FILTERS);
export type BoardFilterValue = z.infer<typeof boardFilterSchema>;

/** `tasks.BoardCountsResponse`. Counts open (`completed_at == null`) tasks in
 * the caller's visible set, independent of any per-column display cap. */
export const boardCountsSchema = z.object({
  all: z.number(),
  mine: z.number(),
  overdue: z.number(),
  today: z.number(),
  unassigned: z.number(),
  by_assignee: z.array(z.object({ teacher_id: z.string(), count: z.number() })),
});
export type BoardCounts = z.infer<typeof boardCountsSchema>;

/** `GET /tasks/board` response. */
export const boardResponseSchema = z.object({
  columns: z.array(boardColumnSchema),
  counts: boardCountsSchema,
});
export type BoardResponse = z.infer<typeof boardResponseSchema>;

/** `PUT /task-columns/order` response. */
export const reorderColumnsResponseSchema = z.object({
  columns: z.array(taskColumnSchema),
});

/** `DELETE /task-columns/:id` response. */
export const deleteColumnResponseSchema = z.object({
  moved_count: z.number(),
});

// --- Request payload shapes (server-validated; no client-side Zod needed
// beyond the react-hook-form resolver for the task form itself). ---

export interface CreateColumnInput {
  name: string;
  is_done?: boolean;
  color?: ColumnColor;
}

export interface UpdateColumnInput {
  name?: string;
  is_done?: boolean;
  color?: ColumnColor;
}

export interface CreateTaskInput {
  title: string;
  description?: string;
  column_id?: string;
  assignee_id?: string;
  priority?: TaskPriority;
  due_on?: string;
}

export interface UpdateTaskInput {
  title?: string;
  description?: string;
  assignee_id?: string | null;
  priority?: TaskPriority;
  due_on?: string | null;
}

/**
 * `POST /tasks/:id/move` body. `after_task_id` names the task the moved one
 * lands *after* inside `column_id`; `null` means "top of the column". The
 * server computes the midpoint position and renormalizes when it needs to.
 */
export interface MoveTaskRequest {
  column_id: string;
  after_task_id: string | null;
}

// --- Task form (react-hook-form + Zod) ---

export const taskFormSchema = z.object({
  title: z.string().trim().min(1, "Vui lòng nhập tiêu đề").max(200, "Tiêu đề tối đa 200 ký tự"),
  description: z
    .string()
    .max(20000, "Mô tả quá dài, hãy bỏ bớt định dạng")
    .refine((value) => plainTextLength(value) <= 2000, "Mô tả tối đa 2000 ký tự"),
  column_id: z.string().min(1, "Vui lòng chọn cột"),
  assignee_id: z.string().nullable(),
  priority: taskPrioritySchema,
  // Kept as a plain "" | "YYYY-MM-DD" string to match the native date input;
  // mapped to `string | null` at the API-payload boundary.
  due_on: z.string(),
});
export type TaskFormValues = z.infer<typeof taskFormSchema>;
