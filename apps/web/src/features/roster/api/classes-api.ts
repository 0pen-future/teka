import { apiClient } from "@/lib/api/client";
import { parseData, parseList, type Paginated } from "@/lib/api/envelope";

import {
  classSchema,
  scheduleSchema,
  type Class,
  type ClassCreateInput,
  type ClassUpdateInput,
  type Schedule,
  type ScheduleInput,
} from "../schemas/roster-schemas";

export interface ListClassesParams {
  status?: "active" | "archived" | "all";
  page?: number;
  per_page?: number;
  sort?: string;
}

/** `GET /classes` (`apps/api/internal/features/classes/handler.go`); active-only by default. */
export async function listClasses(params: ListClassesParams = {}): Promise<Paginated<Class>> {
  const res = await apiClient.get<unknown>("/classes", { params });
  return parseList(classSchema, res.data);
}

export async function getClass(id: string): Promise<Class> {
  const res = await apiClient.get<unknown>(`/classes/${id}`);
  return parseData(classSchema, res.data);
}

/**
 * `POST /classes` creates the class and its weekly schedules atomically —
 * `CreateClassRequest.schedules` is `binding:"required,min=1"` server-side, a
 * class with no timetable would generate no sessions.
 */
export async function createClass(input: ClassCreateInput): Promise<Class> {
  const res = await apiClient.post<unknown>("/classes", input);
  return parseData(classSchema, res.data);
}

/** `PUT /classes/:id` edits name/dates/price only; status and schedules are separate endpoints. */
export async function updateClass(id: string, input: ClassUpdateInput): Promise<Class> {
  const res = await apiClient.put<unknown>(`/classes/${id}`, input);
  return parseData(classSchema, res.data);
}

export async function addSchedule(classId: string, input: ScheduleInput): Promise<Schedule> {
  const res = await apiClient.post<unknown>(`/classes/${classId}/schedules`, input);
  return parseData(scheduleSchema, res.data);
}

export interface UpdateScheduleInput extends ScheduleInput {
  /** Required on update, unlike create where it defaults to the class start date. */
  effective_from: string;
}

export async function updateSchedule(
  classId: string,
  scheduleId: string,
  input: UpdateScheduleInput,
): Promise<Schedule> {
  const res = await apiClient.put<unknown>(`/classes/${classId}/schedules/${scheduleId}`, input);
  return parseData(scheduleSchema, res.data);
}

export async function deleteSchedule(classId: string, scheduleId: string): Promise<void> {
  await apiClient.delete(`/classes/${classId}/schedules/${scheduleId}`);
}
