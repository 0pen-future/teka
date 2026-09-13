import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  boardResponseSchema,
  taskSchema,
  type BoardResponse,
  type BoardScope,
  type CreateTaskInput,
  type Task,
  type UpdateTaskInput,
} from "../schemas/task-schemas";

export interface GetBoardParams {
  scope: BoardScope;
}

export async function getBoard(params: GetBoardParams): Promise<BoardResponse> {
  const res = await apiClient.get<unknown>("/tasks/board", { params });
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

export async function moveTask(taskId: string, columnId: string): Promise<Task> {
  const res = await apiClient.post<unknown>(`/tasks/${taskId}/move`, { column_id: columnId });
  return parseData(taskSchema, res.data);
}

export async function deleteTask(taskId: string): Promise<void> {
  await apiClient.delete(`/tasks/${taskId}`);
}
