import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  assignMemberRole,
  getCenterPermissions,
  isStaleConflict,
  replaceMemberOverrides,
  replaceRolePermissions,
} from "../api/permission-api";
import { centerKeys } from "./use-center";

export const permissionKeys = {
  all: ["center", "permissions"] as const,
};

/** Owner-only read model; callers gate on the owner shape before mounting. */
export function useCenterPermissions() {
  return useQuery({ queryKey: permissionKeys.all, queryFn: getCenterPermissions });
}

/**
 * Every permission mutation invalidates both the permission read model and
 * `/centers/me`: role and override edits change members' effective
 * permission arrays (and the reports.send dual-write flips the roster's
 * send-reports badges). Permission edits are rare, so plain invalidation
 * beats optimistic bookkeeping. A 409 (stale CAS versions) also invalidates:
 * the fresh read model is exactly what the owner must review before
 * re-saving — the component keeps its draft and never auto-retries.
 */
function usePermissionMutation<TVariables>(mutationFn: (variables: TVariables) => Promise<void>) {
  const queryClient = useQueryClient();
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: permissionKeys.all });
    void queryClient.invalidateQueries({ queryKey: centerKeys.me });
  };
  return useMutation({
    mutationFn,
    onSuccess: invalidate,
    onError: (err) => {
      if (isStaleConflict(err)) {
        invalidate();
      }
    },
  });
}

export function useReplaceRolePermissions() {
  return usePermissionMutation(
    ({
      roleId,
      permissions,
      catalogVersion,
      assignmentVersion,
    }: {
      roleId: string;
      permissions: string[];
      catalogVersion: number;
      assignmentVersion: number;
    }) => replaceRolePermissions(roleId, permissions, catalogVersion, assignmentVersion),
  );
}

export function useAssignMemberRole() {
  return usePermissionMutation(({ teacherId, roleId }: { teacherId: string; roleId: string }) =>
    assignMemberRole(teacherId, roleId),
  );
}

export function useReplaceMemberOverrides() {
  return usePermissionMutation(
    ({
      teacherId,
      grants,
      denies,
      catalogVersion,
      assignmentVersion,
    }: {
      teacherId: string;
      grants: string[];
      denies: string[];
      catalogVersion: number;
      assignmentVersion: number;
    }) => replaceMemberOverrides(teacherId, grants, denies, catalogVersion, assignmentVersion),
  );
}
