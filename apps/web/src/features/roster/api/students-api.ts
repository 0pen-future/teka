import { apiClient } from "@/lib/api/client";
import { parseData, parseList, type Paginated } from "@/lib/api/envelope";

import { studentSchema, type Student, type StudentInput } from "../schemas/roster-schemas";

export interface ListStudentsParams {
  query?: string;
  contact_id?: string;
  class_id?: string;
  /** Only students with no open enrollment in any class — the "Chưa ghi danh" tab. */
  unenrolled?: boolean;
  page?: number;
  per_page?: number;
  sort?: string;
}

/** `GET /students` (`apps/api/internal/features/students/handler.go`). */
export async function listStudents(params: ListStudentsParams = {}): Promise<Paginated<Student>> {
  const res = await apiClient.get<unknown>("/students", { params });
  return parseList(studentSchema, res.data);
}

export async function getStudent(id: string): Promise<Student> {
  const res = await apiClient.get<unknown>(`/students/${id}`);
  return parseData(studentSchema, res.data);
}

export async function createStudent(input: StudentInput): Promise<Student> {
  const res = await apiClient.post<unknown>("/students", input);
  return parseData(studentSchema, res.data);
}

/** `PUT /students/:id` — full replace, there is no PATCH on this resource. */
export async function updateStudent(id: string, input: StudentInput): Promise<Student> {
  const res = await apiClient.put<unknown>(`/students/${id}`, input);
  return parseData(studentSchema, res.data);
}

/**
 * `DELETE /students/:id` is anonymize semantics server-side: it scrubs the
 * student's personal data and ends open enrollments, but keeps the
 * anonymized financial records reachable from collections. It is not a row
 * delete.
 */
export async function anonymizeStudent(id: string): Promise<void> {
  await apiClient.delete(`/students/${id}`);
}
