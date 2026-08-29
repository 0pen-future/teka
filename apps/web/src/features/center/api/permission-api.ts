import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import { centerPermissionsSchema, type CenterPermissions } from "../schemas/permission-schemas";

export async function getCenterPermissions(): Promise<CenterPermissions> {
  const res = await apiClient.get<unknown>("/centers/me/permissions");
  return parseData(centerPermissionsSchema, res.data);
}

/**
 * Replace semantics on every mutation (the API keeps no per-key PATCH
 * routes): callers always send the full new set. All three answer 204, so
 * hooks re-read the read model instead of parsing a body.
 */
export async function replaceRolePermissions(roleId: string, permissions: string[]): Promise<void> {
  await apiClient.put(`/centers/me/roles/${roleId}/permissions`, { permissions });
}

export async function assignMemberRole(teacherId: string, roleId: string): Promise<void> {
  await apiClient.put(`/centers/me/members/${teacherId}/role`, { role_id: roleId });
}

export async function replaceMemberOverrides(
  teacherId: string,
  grants: string[],
  denies: string[],
): Promise<void> {
  await apiClient.put(`/centers/me/members/${teacherId}/overrides`, { grants, denies });
}
