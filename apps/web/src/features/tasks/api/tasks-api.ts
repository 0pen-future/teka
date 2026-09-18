import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  boardResponseSchema,
  taskSchema,
  type BoardResponse,
  type BoardFilterValue,
  type CreateTaskInput,
  type MoveTaskRequest,
  type Task,
  type UpdateTaskInput,
} from "../schemas/task-schemas";

export interface GetBoardParams {
  /** The caller's local calendar date (`YYYY-MM-DD`), so "today"/"overdue" match their timezone, not the server's. */
  today: string;
  /** `"all"` (the default) is equivalent to omitting the param server-side; kept here so it is always a defined key on the query cache. */
  filter?: BoardFilterValue;
  /** A teacher's id, ANDed onto `filter` server-side. Omitted (not `""`) when not narrowing by assignee. */
  assignee?: string;
}

/**
 * `filter: "all"` and an empty `assignee` are the server's own defaults, so
 * they are left off the querystring entirely rather than sent as literal
 * values — one less thing for the API contract to special-case.
 */
export async function getBoard(params: GetBoardParams): Promise<BoardResponse> {
  const res = await apiClient.get<unknown>("/tasks/board", {
    params: {
      today: params.today,
      filter: params.filter && params.filter !== "all" ? params.filter : undefined,
      assignee: params.assignee === "" ? undefined : params.assignee,
    },
  });
  return parseData(boardResponseSchema, res.data);
}

export async function createTask(input: CreateTaskInput): Promise<Task> {
  const res = await apiClient.post<unknown>("/tasks", input);
  return parseData(taskSchema, res.data);
}

export async function getTask(taskId: string): Promise<Task> {
  const res = await apiClient.get<unknown>(`/tasks/${taskId}`);
  return parseData(taskSchema, res.data);
}

export async function updateTask(taskId: string, input: UpdateTaskInput): Promise<Task> {
  const res = await apiClient.patch<unknown>(`/tasks/${taskId}`, input);
  return parseData(taskSchema, res.data);
}

export async function moveTask(taskId: string, body: MoveTaskRequest): Promise<Task> {
  const res = await apiClient.post<unknown>(`/tasks/${taskId}/move`, body);
  return parseData(taskSchema, res.data);
}

export async function deleteTask(taskId: string): Promise<void> {
  await apiClient.delete(`/tasks/${taskId}`);
}

/** Undo window for a just-deleted task; the server rejects it once the window has lapsed. */
export async function restoreTask(taskId: string): Promise<Task> {
  const res = await apiClient.post<unknown>(`/tasks/${taskId}/restore`);
  return parseData(taskSchema, res.data);
}
