import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  deleteColumnResponseSchema,
  reorderColumnsResponseSchema,
  taskColumnSchema,
  type CreateColumnInput,
  type TaskColumn,
  type UpdateColumnInput,
} from "../schemas/task-schemas";

export async function createColumn(input: CreateColumnInput): Promise<TaskColumn> {
  const res = await apiClient.post<unknown>("/task-columns", input);
  return parseData(taskColumnSchema, res.data);
}

export async function updateColumn(
  columnId: string,
  input: UpdateColumnInput,
): Promise<TaskColumn> {
  const res = await apiClient.patch<unknown>(`/task-columns/${columnId}`, input);
  return parseData(taskColumnSchema, res.data);
}

export async function reorderColumns(ids: string[]): Promise<TaskColumn[]> {
  const res = await apiClient.put<unknown>("/task-columns/order", { ids });
  return parseData(reorderColumnsResponseSchema, res.data).columns;
}

export async function deleteColumn(columnId: string, moveTo?: string): Promise<number> {
  const res = await apiClient.delete<unknown>(`/task-columns/${columnId}`, {
    params: moveTo ? { move_to: moveTo } : undefined,
  });
  return parseData(deleteColumnResponseSchema, res.data).moved_count;
}
