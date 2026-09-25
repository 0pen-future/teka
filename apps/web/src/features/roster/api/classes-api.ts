import { apiClient } from "@/lib/api/client";
import { parseData, parseList, type Paginated } from "@/lib/api/envelope";

import {
  classAvailabilitySchema,
  classSchema,
  classStatsSchema,
  courseOptionSchema,
  reassignTeacherResponseSchema,
  scheduleSchema,
  type Class,
  type ClassAvailability,
  type ClassCreateInput,
  type ClassPhase,
  type ClassStats,
  type ClassUpdateInput,
  type CourseOption,
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
  /** Only the classes attached to this course. */
  course_id?: string;
  /** Only the classes open for recruitment: the flag is on and the class has not ended or been archived. */
  recruiting?: boolean;
  page?: number;
  per_page?: number;
  sort?: string;
}

/** `GET /classes` (`apps/api/internal/features/classes/handler.go`); active-only by default. */
export async function listClasses(params: ListClassesParams = {}): Promise<Paginated<Class>> {
  const res = await apiClient.get<unknown>("/classes", { params });
  return parseList(classSchema, res.data);
}

/**
 * `GET /courses?status=active` reduced to picker rows. Roster keeps its own
 * lookup so the class dialog can offer a course without importing the
 * courses feature; the full catalog lives there.
 */
export async function listCourseOptions(): Promise<CourseOption[]> {
  const res = await apiClient.get<unknown>("/courses", {
    params: { status: "active", per_page: 100, sort: "name" },
  });
  return parseList(courseOptionSchema, res.data).items;
}

export interface ClassStatsParams {
  /** Count only the classes open for recruitment, the same set as the list's `recruiting` filter. */
  recruiting?: boolean;
}

/** `GET /classes/stats` — per-phase counts over the classes the caller can read. */
export async function getClassStats(params: ClassStatsParams = {}): Promise<ClassStats> {
  const res = await apiClient.get<unknown>("/classes/stats", { params });
  return parseData(classStatsSchema, res.data);
}

/**
 * `GET /classes/availability` — the center's rooms and members, each flagged
 * free unless another live class meets in an overlapping weekly slot. With
 * no slots everything is free, which doubles as the room list.
 */
export async function getClassAvailability(
  slots: { weekday: number; start_time: string; duration_min: number }[],
  excludeClassId?: string,
): Promise<ClassAvailability> {
  const params = new URLSearchParams();
  for (const slot of slots) {
    params.append("slot", `${slot.weekday}-${slot.start_time}-${slot.duration_min}`);
  }
  if (excludeClassId) params.set("exclude_class_id", excludeClassId);
  const res = await apiClient.get<unknown>("/classes/availability", { params });
  return parseData(classAvailabilitySchema, res.data);
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
