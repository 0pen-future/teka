import { apiClient } from "@/lib/api/client";
import { parseData, parseList, type Paginated } from "@/lib/api/envelope";

import {
  enrollmentSchema,
  type Enrollment,
  type EnrollmentCreateInput,
} from "../schemas/roster-schemas";

export interface ListEnrollmentsParams {
  student_id?: string;
  class_id?: string;
  active?: boolean;
  page?: number;
  per_page?: number;
  sort?: string;
}

/** `GET /enrollments` (`apps/api/internal/features/enrollments/handler.go`). */
export async function listEnrollments(
  params: ListEnrollmentsParams = {},
): Promise<Paginated<Enrollment>> {
  const { active, ...rest } = params;
  const res = await apiClient.get<unknown>("/enrollments", {
    params: { ...rest, active: active === undefined ? undefined : String(active) },
  });
  return parseList(enrollmentSchema, res.data);
}

export async function getEnrollment(id: string): Promise<Enrollment> {
  const res = await apiClient.get<unknown>(`/enrollments/${id}`);
  return parseData(enrollmentSchema, res.data);
}

/**
 * `POST /enrollments` — `unit_price` is never sent; the server copies it from
 * `classes.default_unit_price` (PRD section 4's single V1 pricing model).
 */
export async function createEnrollment(input: EnrollmentCreateInput): Promise<Enrollment> {
  const res = await apiClient.post<unknown>("/enrollments", input);
  return parseData(enrollmentSchema, res.data);
}

/**
 * `POST /enrollments/:id/end`. Ending an already-ended enrollment returns 409
 * so a double-submit cannot move the departure date.
 */
export async function endEnrollment(id: string, endedOn?: string): Promise<Enrollment> {
  const res = await apiClient.post<unknown>(`/enrollments/${id}/end`, {
    ended_on: endedOn ?? "",
  });
  return parseData(enrollmentSchema, res.data);
}

/** For enrollments created by mistake; a student leaving is an end, not a delete. */
export async function deleteEnrollment(id: string): Promise<void> {
  await apiClient.delete(`/enrollments/${id}`);
}
