import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  assignClassStaff,
  listClassStaff,
  removeClassStaff,
  type RemoveClassStaffOptions,
} from "../api/class-staff-api";
import type { ClassStaffAssignInput } from "../schemas/roster-schemas";
import { classStaffKeys } from "./roster-keys";

export { classStaffKeys };

export function useClassStaff(classId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: classStaffKeys.list(classId ?? ""),
    queryFn: () => listClassStaff(classId!),
    enabled: Boolean(classId) && enabled,
  });
}

export function useAssignClassStaff(classId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ClassStaffAssignInput) => assignClassStaff(classId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classStaffKeys.list(classId) });
    },
  });
}

export function useRemoveClassStaff(classId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ staffId, options }: { staffId: string; options?: RemoveClassStaffOptions }) =>
      removeClassStaff(classId, staffId, options),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classStaffKeys.list(classId) });
    },
  });
}
