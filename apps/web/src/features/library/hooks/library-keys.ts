import type { ListTemplatesParams } from "../api/library-api";

/**
 * Query keys for the three library entities. Lessons hang off a version and
 * versions off a template, so a write to a lower level invalidates the
 * levels above it (their counts and draft/published summaries change).
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
};

export const lessonsKeys = {
  all: ["library", "lessons"] as const,
  list: (versionId: string) => [...lessonsKeys.all, "list", versionId] as const,
  details: () => [...lessonsKeys.all, "detail"] as const,
  detail: (id: string) => [...lessonsKeys.details(), id] as const,
};
