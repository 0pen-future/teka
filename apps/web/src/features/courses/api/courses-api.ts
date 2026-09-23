import { apiClient } from "@/lib/api/client";
import { parseArray, parseData, parseList, type Paginated } from "@/lib/api/envelope";

import {
  courseSchema,
  tuitionPackSchema,
  type Course,
  type CourseInput,
  type CourseStatus,
  type TuitionPack,
  type TuitionPackInput,
} from "../schemas/courses-schemas";

export interface ListCoursesParams {
  /** One status; absent lists every status. */
  status?: CourseStatus;
  /** Substring of code or name; the API escapes wildcards. */
  q?: string;
  page?: number;
  per_page?: number;
  /** `name` (default) | `code` | `status` | `created_at`, `-` prefix for descending. */
  sort?: string;
}

/** `GET /courses` (`apps/api/internal/features/courses/handler.go`) — center-wide, paginated. */
export async function listCourses(params: ListCoursesParams = {}): Promise<Paginated<Course>> {
  const res = await apiClient.get<unknown>("/courses", { params });
  return parseList(courseSchema, res.data);
}

export async function getCourse(id: string): Promise<Course> {
  const res = await apiClient.get<unknown>(`/courses/${id}`);
  return parseData(courseSchema, res.data);
}

/** `POST /courses` — a live code clash is 409 `CODE_TAKEN`. */
export async function createCourse(input: CourseInput): Promise<Course> {
  const res = await apiClient.post<unknown>("/courses", input);
  return parseData(courseSchema, res.data);
}

/** `PUT /courses/:id` — full replace of the course's own fields; packs have their own call. */
export async function updateCourse(id: string, input: CourseInput): Promise<Course> {
  const res = await apiClient.put<unknown>(`/courses/${id}`, input);
  return parseData(courseSchema, res.data);
}

/** `DELETE /courses/:id` — refused with 409 `COURSE_IN_USE` while live classes point at it. */
export async function deleteCourse(id: string): Promise<void> {
  await apiClient.delete(`/courses/${id}`);
}

/** `POST /courses/:id/archive` — 409 `COURSE_ARCHIVED` when already archived. */
export async function archiveCourse(id: string): Promise<Course> {
  const res = await apiClient.post<unknown>(`/courses/${id}/archive`);
  return parseData(courseSchema, res.data);
}

/** `PUT /courses/:id/tuition-packs` — replaces the whole list (at most 20) in order. */
export async function setTuitionPacks(
  id: string,
  packs: TuitionPackInput[],
): Promise<TuitionPack[]> {
  const res = await apiClient.put<unknown>(`/courses/${id}/tuition-packs`, packs);
  return parseArray(tuitionPackSchema, res.data);
}
