import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createEnrollment,
  deleteEnrollment,
  endEnrollment,
  getEnrollment,
  listEnrollments,
  searchEnrollableStudents,
  type ListEnrollmentsParams,
} from "../api/enrollments-api";
import type { EnrollmentCreateInput } from "../schemas/roster-schemas";
import { classesKeys, enrollmentsKeys, studentsKeys } from "./roster-keys";

export { enrollmentsKeys };

export function useEnrollmentsList(
  params: ListEnrollmentsParams = {},
  options: { enabled?: boolean } = {},
) {
  return useQuery({
    queryKey: enrollmentsKeys.list(params),
    queryFn: () => listEnrollments(params),
    placeholderData: keepPreviousData,
    enabled: options.enabled ?? true,
  });
}

export function useEnrollment(id: string | undefined) {
  return useQuery({
    queryKey: enrollmentsKeys.detail(id ?? ""),
    queryFn: () => getEnrollment(id!),
    enabled: Boolean(id),
  });
}

/**
 * The enrollable-student autocomplete. The two-character minimum mirrors the
 * server (which answers shorter queries with an empty list), so the query
 * only fires once it can return something; `keepPreviousData` keeps the last
 * result list on screen between keystrokes.
 */
export function useEnrollableStudents(classId: string | undefined, q: string) {
  return useQuery({
    queryKey: enrollmentsKeys.enrollable(classId ?? "", q),
    queryFn: () => searchEnrollableStudents(classId!, q),
    enabled: Boolean(classId) && q.trim().length >= 2,
    placeholderData: keepPreviousData,
  });
}

/**
 * Create and end both touch the same surfaces: the class's enrollment
 * list, the enrolled student's detail (its enrollment list refetches), the
 * class detail — per the Architecture's cache invalidation graph — and every
 * students list, whose class/unenrolled filters are derived from enrollments
 * (the app-wide 30s staleTime would otherwise keep a just-enrolled student
 * off the class tab it navigates to).
 */
function invalidateEnrollmentSurfaces(
  queryClient: ReturnType<typeof useQueryClient>,
  studentId: string,
  classId: string,
) {
  void queryClient.invalidateQueries({ queryKey: enrollmentsKeys.lists() });
  void queryClient.invalidateQueries({ queryKey: studentsKeys.lists() });
  void queryClient.invalidateQueries({ queryKey: studentsKeys.detail(studentId) });
  void queryClient.invalidateQueries({ queryKey: classesKeys.detail(classId) });
}

export function useCreateEnrollment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: EnrollmentCreateInput) => createEnrollment(input),
    onSuccess: (enrollment) => {
      invalidateEnrollmentSurfaces(queryClient, enrollment.student_id, enrollment.class_id);
    },
  });
}

export function useEndEnrollment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, endedOn }: { id: string; endedOn?: string }) => endEnrollment(id, endedOn),
    onSuccess: (enrollment) => {
      invalidateEnrollmentSurfaces(queryClient, enrollment.student_id, enrollment.class_id);
    },
  });
}

/** For enrollments created by mistake; a student leaving is an end, not a delete. */
export function useDeleteEnrollment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteEnrollment(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: enrollmentsKeys.all });
      void queryClient.invalidateQueries({ queryKey: studentsKeys.details() });
      void queryClient.invalidateQueries({ queryKey: classesKeys.details() });
    },
  });
}
