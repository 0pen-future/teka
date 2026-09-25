import type { ListItemsParams, ListTemplatesParams } from "../api/library-api";

/**
 * Query keys for the library entities. Lessons hang off a version and
 * versions off a template, so a write to a lower level invalidates the
 * levels above it (their counts and draft/published summaries change).
 * Materials and exercises are center-wide catalogs that lessons link to,
 * so editing one also stales every cached lesson and version detail.
 */
export const templatesKeys = {
  all: ["library", "templates"] as const,
  lists: () => [...templatesKeys.all, "list"] as const,
  list: (params: ListTemplatesParams) => [...templatesKeys.lists(), params] as const,
  details: () => [...templatesKeys.all, "detail"] as const,
  detail: (id: string) => [...templatesKeys.details(), id] as const,
};

export const versionsKeys = {
  all: ["library", "versions"] as const,
  list: (templateId: string) => [...versionsKeys.all, "list", templateId] as const,
  details: () => [...versionsKeys.all, "detail"] as const,
  detail: (id: string) => [...versionsKeys.details(), id] as const,
};

export const lessonsKeys = {
  all: ["library", "lessons"] as const,
  list: (versionId: string) => [...lessonsKeys.all, "list", versionId] as const,
  details: () => [...lessonsKeys.all, "detail"] as const,
  detail: (id: string) => [...lessonsKeys.details(), id] as const,
};

export const materialsKeys = {
  all: ["library", "materials"] as const,
  lists: () => [...materialsKeys.all, "list"] as const,
  list: (params: ListItemsParams) => [...materialsKeys.lists(), params] as const,
};

export const exercisesKeys = {
  all: ["library", "exercises"] as const,
  lists: () => [...exercisesKeys.all, "list"] as const,
  list: (params: ListItemsParams) => [...exercisesKeys.lists(), params] as const,
};

export const exerciseGroupsKeys = {
  all: ["library", "exercise-groups"] as const,
  list: (versionId: string) => [...exerciseGroupsKeys.all, "list", versionId] as const,
};
