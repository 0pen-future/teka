import { apiClient } from "@/lib/api/client";
import { parseData, parseList, type Paginated } from "@/lib/api/envelope";

import {
  classSchema,
  classStatsSchema,
  reassignTeacherResponseSchema,
  scheduleSchema,
  type Class,
  type ClassCreateInput,
  type ClassPhase,
  type ClassStats,
  type ClassUpdateInput,
  type ReassignTeacherResponse,
  type Schedule,
  type ScheduleInput,
} from "../schemas/roster-schemas";

export type ClassShift = "morning" | "afternoon" | "evening";

export interface ListClassesParams {
  status?: "active" | "archived" | "all";
  /** Lifecycle bucket; the API derives it from status and dates, the client never does. */
  phase?: ClassPhase;
  /** Substring of name or code, matched literally (the API escapes wildcards). */
  q?: string;
  /** 0 = Chủ nhật … 6 = Thứ 7; matches a schedule row effective today. */
  weekday?: number;
  shift?: ClassShift;
  /** Exact tag membership. */
  tag?: string;
  page?: number;
  per_page?: number;
  sort?: string;
}

/** `GET /classes` (`apps/api/internal/features/classes/handler.go`); active-only by default. */
export async function listClasses(params: ListClassesParams = {}): Promise<Paginated<Class>> {
  const res = await apiClient.get<unknown>("/classes", { params });
  return parseList(classSchema, res.data);
}

/** `GET /classes/stats` — per-phase counts over the classes the caller can read. */
export async function getClassStats(): Promise<ClassStats> {
  const res = await apiClient.get<unknown>("/classes/stats");
  return parseData(classStatsSchema, res.data);
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

/**
 * `PUT /classes/:id` edits name/dates/price plus the catalog fields (code,
 * tags, recruiting, note — each optional, absent = unchanged); status and
 * schedules are separate endpoints.
 */
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

/**
 * `PUT /classes/:id/teacher` (`apps/api/internal/features/handoff`) — owner-only
 * handoff. Moves the class, its schedules and its future planned sessions to
 * `teacherId`; held/past/cancelled sessions and billing history stay behind.
 */
export async function reassignTeacher(
  classId: string,
  teacherId: string,
): Promise<ReassignTeacherResponse> {
  const res = await apiClient.put<unknown>(`/classes/${classId}/teacher`, {
    teacher_id: teacherId,
  });
  return parseData(reassignTeacherResponseSchema, res.data);
}
