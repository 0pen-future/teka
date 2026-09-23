import type { ListPathsParams } from "../api/paths-api";

export const pathsKeys = {
  all: ["paths"] as const,
  lists: () => [...pathsKeys.all, "list"] as const,
  list: (params: ListPathsParams) => [...pathsKeys.lists(), params] as const,
  details: () => [...pathsKeys.all, "detail"] as const,
  detail: (id: string) => [...pathsKeys.details(), id] as const,
  /** Paths that recommend one course, read from the course detail page. */
  byCourse: (courseId: string) => [...pathsKeys.all, "by-course", courseId] as const,
};
