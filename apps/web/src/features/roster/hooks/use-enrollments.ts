import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createEnrollment,
  deleteEnrollment,
  endEnrollment,
  getEnrollment,
  listEnrollments,
  type ListEnrollmentsParams,
} from "../api/enrollments-api";
import type { EnrollmentCreateInput } from "../schemas/roster-schemas";
import { classesKeys, enrollmentsKeys, studentsKeys } from "./roster-keys";

export { enrollmentsKeys };

export function useEnrollmentsList(params: ListEnrollmentsParams = {}) {
  return useQuery({
    queryKey: enrollmentsKeys.list(params),
    queryFn: () => listEnrollments(params),
    placeholderData: keepPreviousData,
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
 * Create and end both touch the same three surfaces: the class's enrollment
 * list, the enrolled student's detail (its enrollment list refetches), and
 * the class detail — per the Architecture's cache invalidation graph.
 */
function invalidateEnrollmentSurfaces(
  queryClient: ReturnType<typeof useQueryClient>,
  studentId: string,
  classId: string,
) {
  void queryClient.invalidateQueries({ queryKey: enrollmentsKeys.lists() });
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
