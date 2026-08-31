import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";
import { ApiError } from "@/lib/api/errors";

import { centerPermissionsSchema, type CenterPermissions } from "../schemas/permission-schemas";

export async function getCenterPermissions(): Promise<CenterPermissions> {
  const res = await apiClient.get<unknown>("/centers/me/permissions");
  return parseData(centerPermissionsSchema, res.data);
}

/**
 * True when a replacement write lost the compare-and-set race: someone else
 * saved since this client's read. The only cure is a refetch and a reviewed
 * re-save — callers must never auto-retry the same payload.
 */
export function isStaleConflict(err: unknown): boolean {
  return err instanceof ApiError && err.status === 409;
}

/**
 * Replace semantics on every mutation (the API keeps no per-key PATCH
 * routes): callers always send the full new set, plus the catalog and
 * assignment versions of the read model the edit was composed on — a stale
 * pair 409s without mutating. All three answer 204, so hooks re-read the
 * read model instead of parsing a body.
 */
export async function replaceRolePermissions(
  roleId: string,
  permissions: string[],
  catalogVersion: number,
  assignmentVersion: number,
): Promise<void> {
  await apiClient.put(`/centers/me/roles/${roleId}/permissions`, {
    permissions,
    catalog_version: catalogVersion,
    assignment_version: assignmentVersion,
  });
}

export async function assignMemberRole(teacherId: string, roleId: string): Promise<void> {
  await apiClient.put(`/centers/me/members/${teacherId}/role`, { role_id: roleId });
}

export async function replaceMemberOverrides(
  teacherId: string,
  grants: string[],
  denies: string[],
  catalogVersion: number,
  assignmentVersion: number,
): Promise<void> {
  await apiClient.put(`/centers/me/members/${teacherId}/overrides`, {
    grants,
    denies,
    catalog_version: catalogVersion,
    assignment_version: assignmentVersion,
  });
}
