import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { teachingKeys } from "@/features/teaching";

import {
  applyClassProgram,
  getClassProgram,
  getCourseDefaultTemplate,
  listClassProgramLessons,
  removeClassProgram,
} from "../api/class-program-api";
import type { ApplyClassProgramInput } from "../schemas/class-program-schemas";
import { classProgramKeys } from "./roster-keys";

export function useClassProgram(classId: string) {
  return useQuery({
    queryKey: classProgramKeys.detail(classId),
    queryFn: () => getClassProgram(classId),
  });
}

/** The applied version's lessons with materials and exercises; `enabled` skips the read while no program exists. */
export function useClassProgramLessons(classId: string, enabled = true) {
  return useQuery({
    queryKey: classProgramKeys.lessons(classId),
    queryFn: () => listClassProgramLessons(classId),
    enabled,
  });
}

export function useCourseDefaultTemplate(courseId: string | undefined) {
  return useQuery({
    queryKey: classProgramKeys.courseDefault(courseId ?? ""),
    queryFn: () => getCourseDefaultTemplate(courseId ?? ""),
    enabled: Boolean(courseId),
  });
}

/**
 * Applying or swapping rewrites the class curriculum's lesson titles
 * server-side, so the classbook's curriculum cache is stale too. Removing
 * only drops the link, but the same invalidation keeps both paths uniform.
 */
function useProgramWrite<TVars>(classId: string, mutationFn: (vars: TVars) => Promise<unknown>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classProgramKeys.detail(classId) });
      void queryClient.invalidateQueries({ queryKey: classProgramKeys.lessons(classId) });
      void queryClient.invalidateQueries({ queryKey: teachingKeys.curriculum(classId) });
    },
  });
}

export function useApplyClassProgram(classId: string) {
  return useProgramWrite(classId, (input: ApplyClassProgramInput) =>
    applyClassProgram(classId, input),
  );
}

export function useRemoveClassProgram(classId: string) {
  return useProgramWrite(classId, () => removeClassProgram(classId));
}
