import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { sessionsKeys } from "@/features/attendance";

import {
  addSchedule,
  createClass,
  deleteSchedule,
  getClass,
  listClasses,
  reassignTeacher,
  updateClass,
  updateSchedule,
  type ListClassesParams,
  type UpdateScheduleInput,
} from "../api/classes-api";
import type { ClassCreateInput, ClassUpdateInput, ScheduleInput } from "../schemas/roster-schemas";
import { classesKeys } from "./roster-keys";

export { classesKeys };

export function useClassesList(params: ListClassesParams = {}) {
  return useQuery({
    queryKey: classesKeys.list(params),
    queryFn: () => listClasses(params),
    placeholderData: keepPreviousData,
  });
}

export function useClass(id: string | undefined) {
  return useQuery({
    queryKey: classesKeys.detail(id ?? ""),
    queryFn: () => getClass(id!),
    enabled: Boolean(id),
  });
}

export function useCreateClass() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ClassCreateInput) => createClass(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classesKeys.lists() });
    },
  });
}

export function useUpdateClass(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ClassUpdateInput) => updateClass(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classesKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: classesKeys.detail(id) });
    },
  });
}

/**
 * Owner-only teacher handoff. The reassignment moves the class row, its
 * schedules and its future planned sessions server-side, so both the class
 * caches and every session list are stale afterward — invalidate broadly
 * rather than track which dates moved.
 */
export function useReassignTeacher(classId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (teacherId: string) => reassignTeacher(classId, teacherId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classesKeys.all });
      void queryClient.invalidateQueries({ queryKey: sessionsKeys.all });
    },
  });
}

/**
 * Schedule mutations invalidate only the owning class's detail — session
 * generation is server-side and phase 3's attendance screens refetch on
 * their own, per the Architecture's cache invalidation graph.
 */
export function useAddSchedule(classId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ScheduleInput) => addSchedule(classId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classesKeys.detail(classId) });
    },
  });
}

export function useUpdateSchedule(classId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ scheduleId, input }: { scheduleId: string; input: UpdateScheduleInput }) =>
      updateSchedule(classId, scheduleId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classesKeys.detail(classId) });
    },
  });
}

export function useDeleteSchedule(classId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (scheduleId: string) => deleteSchedule(classId, scheduleId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classesKeys.detail(classId) });
    },
  });
}
