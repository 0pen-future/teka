import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  anonymizeStudent,
  createStudent,
  getStudent,
  listStudents,
  updateStudent,
  type ListStudentsParams,
} from "../api/students-api";
import type { StudentInput } from "../schemas/roster-schemas";
import { contactsKeys, enrollmentsKeys, studentsKeys } from "./roster-keys";

export { studentsKeys };

export function useStudentsList(params: ListStudentsParams = {}) {
  return useQuery({
    queryKey: studentsKeys.list(params),
    queryFn: () => listStudents(params),
    placeholderData: keepPreviousData,
  });
}

export function useStudent(id: string | undefined) {
  return useQuery({
    queryKey: studentsKeys.detail(id ?? ""),
    queryFn: () => getStudent(id!),
    enabled: Boolean(id),
  });
}

export function useCreateStudent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: StudentInput) => createStudent(input),
    onSuccess: (student) => {
      void queryClient.invalidateQueries({ queryKey: studentsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: studentsKeys.detail(student.id) });
      // student_count on the owning contact just changed.
      void queryClient.invalidateQueries({ queryKey: contactsKeys.all });
    },
  });
}

export function useUpdateStudent(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: StudentInput) => updateStudent(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: studentsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: studentsKeys.detail(id) });
      // A reassigned contact_id changes student_count on both the old and new
      // contact; invalidating every contact query is simpler and cheap for a
      // V1 roster size than tracking which two contacts were involved.
      void queryClient.invalidateQueries({ queryKey: contactsKeys.all });
    },
  });
}

/**
 * `DELETE /students/:id` is anonymize semantics: it scrubs personal data and
 * ends the student's open enrollments server-side, so the enrollment lists
 * need invalidating alongside the student and contact caches.
 */
export function useAnonymizeStudent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => anonymizeStudent(id),
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({ queryKey: studentsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: studentsKeys.detail(id) });
      void queryClient.invalidateQueries({ queryKey: contactsKeys.all });
      void queryClient.invalidateQueries({ queryKey: enrollmentsKeys.all });
    },
  });
}
