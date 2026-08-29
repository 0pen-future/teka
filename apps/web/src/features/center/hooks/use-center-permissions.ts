import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  assignMemberRole,
  getCenterPermissions,
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
 * beats optimistic bookkeeping.
 */
function usePermissionMutation<TVariables>(mutationFn: (variables: TVariables) => Promise<void>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: permissionKeys.all });
      void queryClient.invalidateQueries({ queryKey: centerKeys.me });
    },
  });
}

export function useReplaceRolePermissions() {
  return usePermissionMutation(
    ({ roleId, permissions }: { roleId: string; permissions: string[] }) =>
      replaceRolePermissions(roleId, permissions),
  );
}

export function useAssignMemberRole() {
  return usePermissionMutation(({ teacherId, roleId }: { teacherId: string; roleId: string }) =>
    assignMemberRole(teacherId, roleId),
  );
}

export function useReplaceMemberOverrides() {
  return usePermissionMutation(
    ({ teacherId, grants, denies }: { teacherId: string; grants: string[]; denies: string[] }) =>
      replaceMemberOverrides(teacherId, grants, denies),
  );
}
