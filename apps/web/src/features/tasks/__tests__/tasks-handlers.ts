import { http, HttpResponse } from "msw";

import { API_URL, fail, ok, primaryTeacher, secondaryTeacher } from "@/test/msw/handlers";

// Stateful mock backend for board-level flow tests (create/move/delete,
// column CRUD + reorder + delete-with-move). Single-shot error scenarios
// that don't need persisted state are simpler to declare inline with
// `server.use()` in the test file that needs them.

export const columnTodoId = "60000000-0000-4000-8000-000000000001";
export const columnDoneId = "60000000-0000-4000-8000-000000000002";

/** Created and assigned to the signed-in owner — the editable-form case. */
export const ownTaskId = "61000000-0000-4000-8000-000000000001";
/** Created by the owner but assigned to someone else — still editable by the creator. */
export const assignedToOthersTaskId = "61000000-0000-4000-8000-000000000002";
/** Created by someone else and assigned to the signed-in teacher — the read-only-form case. */
export const foreignTaskId = "61000000-0000-4000-8000-000000000003";

const MAX_COLUMNS = 8;

interface MutableColumn {
  id: string;
  name: string;
  position: number;
  is_done: boolean;
  created_at: string;
  updated_at: string;
}

interface MutableTask {
  id: string;
  column_id: string;
  title: string;
  description: string;
  priority: string;
  due_on: string | null;
  assignee_id: string | null;
  created_by: string;
  position: number;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
}

let columns: MutableColumn[] = [];
let tasks: MutableTask[] = [];
/** Rows `DELETE /tasks/:id` moved out of `tasks`, so `POST /tasks/:id/restore` can bring them back. */
let deletedTasks: MutableTask[] = [];
let columnIdCounter = 0;
let taskIdCounter = 0;

/** Every `GET /tasks/board` request's `today` query param, for assertions. */
export const boardTodayRequests: string[] = [];
/** Every `PUT /task-columns/order` request body, for assertions. */
export const columnOrderRequests: string[][] = [];
/** Every `POST /tasks` and `PATCH /tasks/:id` request body, for assertions on what the form sends. */
export const taskWriteRequests: Record<string, unknown>[] = [];

/** Overwrites a seeded task's description, for cases the default fixtures do not cover. */
export function seedTaskDescription(taskId: string, description: string): void {
  const task = tasks.find((candidate) => candidate.id === taskId);
  if (task) task.description = description;
}

export function resetTasksStore(): void {
  columnIdCounter = 0;
  taskIdCounter = 0;
  boardTodayRequests.length = 0;
  columnOrderRequests.length = 0;
  taskWriteRequests.length = 0;
  deletedTasks = [];
  columns = [
    {
      id: columnTodoId,
      name: "Cần làm",
      position: 1,
      is_done: false,
      created_at: "2026-09-01T08:00:00Z",
      updated_at: "2026-09-01T08:00:00Z",
    },
    {
      id: columnDoneId,
      name: "Hoàn thành",
      position: 2,
      is_done: true,
      created_at: "2026-09-01T08:00:00Z",
      updated_at: "2026-09-01T08:00:00Z",
    },
  ];
  tasks = [
    {
      id: ownTaskId,
      column_id: columnTodoId,
      title: "Soạn đề kiểm tra giữa kỳ",
      description:
        "<p>Đề gồm <strong>3 phần</strong>:</p><ul><li>Trắc nghiệm</li><li>Tự luận</li></ul>",
      priority: "medium",
      due_on: "2026-09-25",
      assignee_id: primaryTeacher.id,
      created_by: primaryTeacher.id,
      position: 1,
      completed_at: null,
      created_at: "2026-09-10T08:00:00Z",
      updated_at: "2026-09-10T08:00:00Z",
    },
    {
      id: assignedToOthersTaskId,
      column_id: columnTodoId,
      title: "Sắp lịch dạy bù",
      // A row migration 000023 has not rewritten yet: still plain text.
      description: "Hỏi lớp 6A & 7B\nbáo lại trước thứ 6",
      priority: "low",
      due_on: null,
      assignee_id: secondaryTeacher.id,
      created_by: primaryTeacher.id,
      position: 2,
      completed_at: null,
      created_at: "2026-09-11T08:00:00Z",
      updated_at: "2026-09-11T08:00:00Z",
    },
    {
      id: foreignTaskId,
      column_id: columnTodoId,
      title: "Kiểm tra học phí tháng 9",
      description:
        '<p>Xem <a href="https://teka.vn/hoc-phi">bảng học phí</a><script>alert(1)</script></p>',
      priority: "high",
      due_on: null,
      assignee_id: primaryTeacher.id,
      created_by: secondaryTeacher.id,
      position: 3,
      completed_at: null,
      created_at: "2026-09-12T08:00:00Z",
      updated_at: "2026-09-12T08:00:00Z",
    },
  ];
}

resetTasksStore();

function boardColumnsPayload() {
  return [...columns]
    .sort((a, b) => a.position - b.position)
    .map((column) => ({
      ...column,
      tasks: tasks
        .filter((task) => task.column_id === column.id)
        .sort((a, b) => a.position - b.position),
      has_more: false,
    }));
}

/** Mirrors `tasks.BoardCountsResponse`: open tasks only, `overdue`/`today` compared against the caller's `today`. */
function boardCounts(today: string) {
  const open = tasks.filter((task) => task.completed_at === null);
  const byAssignee = new Map<string, number>();
  let overdue = 0;
  let dueToday = 0;
  let unassigned = 0;
  open.forEach((task) => {
    if (task.due_on !== null) {
      if (task.due_on < today) overdue += 1;
      else if (task.due_on === today) dueToday += 1;
    }
    if (task.assignee_id === null) {
      unassigned += 1;
    } else {
      byAssignee.set(task.assignee_id, (byAssignee.get(task.assignee_id) ?? 0) + 1);
    }
  });
  const by_assignee = [...byAssignee.entries()]
    .map(([teacher_id, count]) => ({ teacher_id, count }))
    .sort((a, b) => b.count - a.count || a.teacher_id.localeCompare(b.teacher_id));
  return {
    all: open.length,
    mine: byAssignee.get(primaryTeacher.id) ?? 0,
    overdue,
    today: dueToday,
    unassigned,
    by_assignee,
  };
}

export const tasksHandlers = [
  http.get(`${API_URL}/tasks/board`, ({ request }) => {
    const url = new URL(request.url);
    const today = url.searchParams.get("today") ?? "2026-09-17";
    boardTodayRequests.push(today);
    return HttpResponse.json(ok({ columns: boardColumnsPayload(), counts: boardCounts(today) }));
  }),
  http.post(`${API_URL}/task-columns`, async ({ request }) => {
    const body = (await request.json()) as { name: string; is_done?: boolean };
    const trimmed = body.name.trim();
    if (columns.some((column) => column.name === trimmed)) {
      return HttpResponse.json(
        fail("DUPLICATE_COLUMN_NAME", "Tên cột đã tồn tại", { name: "Tên cột đã tồn tại" }),
        { status: 422 },
      );
    }
    if (columns.length >= MAX_COLUMNS) {
      return HttpResponse.json(
        fail("COLUMN_LIMIT", "Đã đạt số cột tối đa", { name: "Đã đạt số cột tối đa" }),
        { status: 422 },
      );
    }
    columnIdCounter += 1;
    const column: MutableColumn = {
      id: `60000000-0000-4000-8000-0000000000${String(90 + columnIdCounter).padStart(2, "0")}`,
      name: trimmed,
      position: columns.length + 1,
      is_done: body.is_done ?? false,
      created_at: "2026-09-13T10:00:00Z",
      updated_at: "2026-09-13T10:00:00Z",
    };
    columns.push(column);
    return HttpResponse.json(ok(column), { status: 201 });
  }),
  http.patch(`${API_URL}/task-columns/:id`, async ({ params, request }) => {
    const body = (await request.json()) as { name?: string; is_done?: boolean };
    const column = columns.find((candidate) => candidate.id === params.id);
    if (!column) {
      return HttpResponse.json(fail("NOT_FOUND", "column not found"), { status: 404 });
    }
    if (body.name !== undefined) {
      const trimmed = body.name.trim();
      if (columns.some((candidate) => candidate.id !== column.id && candidate.name === trimmed)) {
        return HttpResponse.json(
          fail("DUPLICATE_COLUMN_NAME", "Tên cột đã tồn tại", { name: "Tên cột đã tồn tại" }),
          { status: 422 },
        );
      }
      column.name = trimmed;
    }
    if (body.is_done !== undefined) {
      column.is_done = body.is_done;
    }
    column.updated_at = "2026-09-13T10:00:00Z";
    return HttpResponse.json(ok(column));
  }),
  http.put(`${API_URL}/task-columns/order`, async ({ request }) => {
    const body = (await request.json()) as { ids: string[] };
    columnOrderRequests.push(body.ids);
    if (
      body.ids.length !== columns.length ||
      body.ids.some((id) => !columns.some((c) => c.id === id))
    ) {
      return HttpResponse.json(
        fail("INVALID_PERMUTATION", "Thứ tự cột không hợp lệ", { ids: "Thứ tự cột không hợp lệ" }),
        { status: 422 },
      );
    }
    body.ids.forEach((id, index) => {
      const column = columns.find((candidate) => candidate.id === id);
      if (column) {
        column.position = index + 1;
        column.updated_at = "2026-09-13T10:00:00Z";
      }
    });
    return HttpResponse.json(ok({ columns: [...columns].sort((a, b) => a.position - b.position) }));
  }),
  http.delete(`${API_URL}/task-columns/:id`, ({ params, request }) => {
    const column = columns.find((candidate) => candidate.id === params.id);
    if (!column) {
      return HttpResponse.json(fail("NOT_FOUND", "column not found"), { status: 404 });
    }
    if (columns.length <= 1) {
      return HttpResponse.json(fail("LAST_COLUMN", "Không thể xoá cột cuối cùng"), { status: 409 });
    }
    const url = new URL(request.url);
    const moveTo = url.searchParams.get("move_to");
    const columnTasks = tasks.filter((task) => task.column_id === column.id);
    if (columnTasks.length > 0 && !moveTo) {
      return HttpResponse.json(fail("COLUMN_NOT_EMPTY", "Cột còn việc chưa được chuyển"), {
        status: 409,
      });
    }
    if (moveTo) {
      columnTasks.forEach((task) => {
        task.column_id = moveTo;
      });
    }
    columns = columns.filter((candidate) => candidate.id !== column.id);
    return HttpResponse.json(ok({ moved_count: columnTasks.length }));
  }),
  http.post(`${API_URL}/tasks`, async ({ request }) => {
    const body = (await request.json()) as {
      title: string;
      description?: string;
      column_id?: string;
      assignee_id?: string;
      priority?: string;
      due_on?: string;
    };
    taskWriteRequests.push(body);
    taskIdCounter += 1;
    const task: MutableTask = {
      id: `61000000-0000-4000-8000-0000000000${String(90 + taskIdCounter).padStart(2, "0")}`,
      column_id: body.column_id ?? columnTodoId,
      title: body.title,
      description: body.description ?? "",
      priority: body.priority ?? "none",
      due_on: body.due_on ?? null,
      assignee_id: body.assignee_id ?? null,
      created_by: primaryTeacher.id,
      position: 0,
      completed_at: null,
      created_at: "2026-09-13T10:00:00Z",
      updated_at: "2026-09-13T10:00:00Z",
    };
    tasks.forEach((existing) => {
      if (existing.column_id === task.column_id) {
        existing.position += 1;
      }
    });
    tasks.push(task);
    return HttpResponse.json(ok(task), { status: 201 });
  }),
  http.get(`${API_URL}/tasks/:id`, ({ params }) => {
    const task = tasks.find((candidate) => candidate.id === params.id);
    if (!task) {
      return HttpResponse.json(fail("NOT_FOUND", "task not found"), { status: 404 });
    }
    return HttpResponse.json(ok(task));
  }),
  http.patch(`${API_URL}/tasks/:id`, async ({ params, request }) => {
    const body = (await request.json()) as Record<string, unknown>;
    taskWriteRequests.push(body);
    const task = tasks.find((candidate) => candidate.id === params.id);
    if (!task) {
      return HttpResponse.json(fail("NOT_FOUND", "task not found"), { status: 404 });
    }
    Object.assign(task, body, { updated_at: "2026-09-13T10:00:00Z" });
    return HttpResponse.json(ok(task));
  }),
  http.post(`${API_URL}/tasks/:id/move`, async ({ params, request }) => {
    const body = (await request.json()) as { column_id: string; after_task_id?: string | null };
    const task = tasks.find((candidate) => candidate.id === params.id);
    if (!task) {
      return HttpResponse.json(fail("NOT_FOUND", "task not found"), { status: 404 });
    }
    const destination = columns.find((column) => column.id === body.column_id);
    if (!destination) {
      return HttpResponse.json(fail("NOT_FOUND", "column not found"), { status: 404 });
    }
    // Mirrors the real API: land right after `after_task_id` (which must be
    // in the destination column), or at the top when it is absent/null, at
    // the midpoint between the two neighbours.
    const siblings = tasks
      .filter((candidate) => candidate.column_id === body.column_id && candidate.id !== task.id)
      .sort((a, b) => a.position - b.position);
    let prev: MutableTask | undefined;
    let next: MutableTask | undefined;
    if (body.after_task_id) {
      const afterIndex = siblings.findIndex((candidate) => candidate.id === body.after_task_id);
      if (afterIndex === -1) {
        return HttpResponse.json(
          fail("VALIDATION_ERROR", "validation failed", {
            after_task_id: "phải là việc đang nằm trong cột đích",
          }),
          { status: 422 },
        );
      }
      prev = siblings[afterIndex];
      next = siblings[afterIndex + 1];
    } else {
      next = siblings[0];
    }
    task.column_id = body.column_id;
    task.position =
      prev === undefined
        ? next === undefined
          ? 0
          : next.position - 1
        : next === undefined
          ? prev.position + 1
          : (prev.position + next.position) / 2;
    task.completed_at = destination.is_done ? "2026-09-13T10:00:00Z" : null;
    task.updated_at = "2026-09-13T10:00:00Z";
    return HttpResponse.json(ok(task));
  }),
  http.delete(`${API_URL}/tasks/:id`, ({ params }) => {
    const index = tasks.findIndex((candidate) => candidate.id === params.id);
    const removed = index === -1 ? undefined : tasks[index];
    if (!removed) {
      return HttpResponse.json(fail("NOT_FOUND", "task not found"), { status: 404 });
    }
    tasks.splice(index, 1);
    deletedTasks.push(removed);
    return new HttpResponse(null, { status: 204 });
  }),
  http.post(`${API_URL}/tasks/:id/restore`, ({ params }) => {
    const index = deletedTasks.findIndex((candidate) => candidate.id === params.id);
    const restored = index === -1 ? undefined : deletedTasks[index];
    if (!restored) {
      return HttpResponse.json(fail("NOT_FOUND", "task not found"), { status: 404 });
    }
    deletedTasks.splice(index, 1);
    restored.updated_at = "2026-09-13T10:00:00Z";
    tasks.push(restored);
    return HttpResponse.json(ok(restored));
  }),
  http.get(`${API_URL}/centers/me/members/directory`, () =>
    HttpResponse.json(
      ok([
        {
          teacher_id: primaryTeacher.id,
          display_name: primaryTeacher.full_name,
          role_name: "Chủ trung tâm",
        },
        {
          teacher_id: secondaryTeacher.id,
          display_name: secondaryTeacher.full_name,
          role_name: "Giáo viên",
        },
      ]),
    ),
  ),
];
