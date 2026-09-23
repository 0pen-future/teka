import type { ListCoursesParams } from "../api/courses-api";

/** Query keys for the course catalog; a write invalidates the lists and patches the detail it returned. */
export const coursesKeys = {
  all: ["courses"] as const,
  lists: () => [...coursesKeys.all, "list"] as const,
  list: (params: ListCoursesParams) => [...coursesKeys.lists(), params] as const,
  details: () => [...coursesKeys.all, "detail"] as const,
  detail: (id: string) => [...coursesKeys.details(), id] as const,
};
