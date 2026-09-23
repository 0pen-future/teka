import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { classesKeys } from "@/features/roster";

import {
  archiveCourse,
  createCourse,
  deleteCourse,
  getCourse,
  listCourses,
  setTuitionPacks,
  updateCourse,
  type ListCoursesParams,
} from "../api/courses-api";
import type { Course, CourseInput, TuitionPackInput } from "../schemas/courses-schemas";
import { coursesKeys } from "./courses-keys";

export function useCoursesList(params: ListCoursesParams = {}, enabled = true) {
  return useQuery({
    queryKey: coursesKeys.list(params),
    queryFn: () => listCourses(params),
    enabled,
    placeholderData: keepPreviousData,
  });
}

export function useCourse(id: string | undefined) {
  return useQuery({
    queryKey: coursesKeys.detail(id ?? ""),
    queryFn: () => getCourse(id ?? ""),
    enabled: Boolean(id),
  });
}

export function useCreateCourse() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: createCourse,
    onSuccess: (course) => {
      queryClient.setQueryData(coursesKeys.detail(course.id), course);
    },
    // Settled, not success: a request that failed after the server wrote
    // (timeout, dropped connection) must still refresh the list.
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: coursesKeys.lists() });
      // The class dialog's course picker reads the same catalog.
      void queryClient.invalidateQueries({ queryKey: classesKeys.courseOptions() });
    },
  });
}

/** Full replace; the returned course becomes the cached detail so the page never shows stale fields. */
export function useUpdateCourse(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CourseInput) => updateCourse(id, input),
    onSuccess: (course) => {
      queryClient.setQueryData(coursesKeys.detail(id), course);
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: coursesKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: classesKeys.courseOptions() });
      // Classes embed the course code and name, in the list and on the
      // detail header chip alike.
      void queryClient.invalidateQueries({ queryKey: classesKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: classesKeys.details() });
    },
  });
}

export function useDeleteCourse() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deleteCourse,
    onSuccess: (_, id) => {
      queryClient.removeQueries({ queryKey: coursesKeys.detail(id) });
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: coursesKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: classesKeys.courseOptions() });
    },
  });
}

/** Archiving leaves classes attached, so only the catalog surfaces refetch. */
export function useArchiveCourse(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => archiveCourse(id),
    onSuccess: (course: Course) => {
      queryClient.setQueryData(coursesKeys.detail(id), course);
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: coursesKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: classesKeys.courseOptions() });
    },
  });
}

export function useSetTuitionPacks(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (packs: TuitionPackInput[]) => setTuitionPacks(id, packs),
    onSuccess: (packs) => {
      queryClient.setQueryData<Course>(coursesKeys.detail(id), (current) =>
        current ? { ...current, tuition_packs: packs } : current,
      );
      void queryClient.invalidateQueries({ queryKey: coursesKeys.lists() });
    },
  });
}
