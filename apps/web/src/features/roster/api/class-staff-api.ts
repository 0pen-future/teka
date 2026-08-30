import { apiClient } from "@/lib/api/client";
import { parseArray, parseData } from "@/lib/api/envelope";

import {
  classStaffSchema,
  type ClassStaff,
  type ClassStaffAssignInput,
} from "../schemas/roster-schemas";

/**
 * `GET /classes/:id/staff` (`apps/api/internal/features/classstaff/handler.go`)
 * — every stint, active and ended; the API returns no `meta` block.
 */
export async function listClassStaff(classId: string): Promise<ClassStaff[]> {
  const res = await apiClient.get<unknown>(`/classes/${classId}/staff`);
  return parseArray(classStaffSchema, res.data);
}

/** `POST /classes/:id/staff` — owner-only; `giao_vien` is refused server-side (409). */
export async function assignClassStaff(
  classId: string,
  input: ClassStaffAssignInput,
): Promise<ClassStaff> {
  const res = await apiClient.post<unknown>(`/classes/${classId}/staff`, input);
  return parseData(classStaffSchema, res.data);
}

export interface RemoveClassStaffOptions {
  /** Hard-deletes the stint (the mistaken-grant revocation path) instead of soft-closing it. */
  void?: boolean;
}

/**
 * `DELETE /classes/:id/staff/:staffId` — owner-only. Default soft-closes the
 * stint (keeps history reads); `void: true` hard-deletes it.
 */
export async function removeClassStaff(
  classId: string,
  staffId: string,
  options: RemoveClassStaffOptions = {},
): Promise<void> {
  await apiClient.delete(`/classes/${classId}/staff/${staffId}`, {
    params: options.void ? { mode: "void" } : undefined,
  });
}
