import { apiClient } from "@/lib/api/client";
import { parseArray, parseData, parseList, type Paginated } from "@/lib/api/envelope";

import {
  programTemplateSchema,
  templateLessonSchema,
  templateVersionSchema,
  type LessonInput,
  type ProgramTemplate,
  type TemplateInput,
  type TemplateLesson,
  type TemplateVersion,
} from "../schemas/library-schemas";

export interface ListTemplatesParams {
  /** Substring of code or name; the API escapes wildcards. */
  q?: string;
  page?: number;
  per_page?: number;
  /** `name` (default) | `code` | `created_at`, `-` prefix for descending. */
  sort?: string;
}

/** `GET /library/templates` (`apps/api/internal/features/library/handler.go`) — center-wide, paginated. */
export async function listTemplates(
  params: ListTemplatesParams = {},
): Promise<Paginated<ProgramTemplate>> {
  const res = await apiClient.get<unknown>("/library/templates", { params });
  return parseList(programTemplateSchema, res.data);
}

export async function getTemplate(id: string): Promise<ProgramTemplate> {
  const res = await apiClient.get<unknown>(`/library/templates/${id}`);
  return parseData(programTemplateSchema, res.data);
}

/** `POST /library/templates` — opens draft v1 in the same transaction; a live code clash is 409 `CODE_TAKEN`. */
export async function createTemplate(input: TemplateInput): Promise<ProgramTemplate> {
  const res = await apiClient.post<unknown>("/library/templates", input);
  return parseData(programTemplateSchema, res.data);
}

export async function updateTemplate(id: string, input: TemplateInput): Promise<ProgramTemplate> {
  const res = await apiClient.put<unknown>(`/library/templates/${id}`, input);
  return parseData(programTemplateSchema, res.data);
}

/** `DELETE /library/templates/:id` — soft delete; the code becomes reusable. */
export async function deleteTemplate(id: string): Promise<void> {
  await apiClient.delete(`/library/templates/${id}`);
}

export async function listVersions(templateId: string): Promise<TemplateVersion[]> {
  const res = await apiClient.get<unknown>(`/library/templates/${templateId}/versions`);
  return parseArray(templateVersionSchema, res.data);
}

/**
 * `POST /library/templates/:id/versions` — opens the next draft, copying the
 * latest published version's lessons. One draft at a time: 409 `DRAFT_EXISTS`.
 */
export async function createVersion(
  templateId: string,
  changelog: string | null,
): Promise<TemplateVersion> {
  const res = await apiClient.post<unknown>(`/library/templates/${templateId}/versions`, {
    changelog,
  });
  return parseData(templateVersionSchema, res.data);
}

/** `POST /library/versions/:vid/publish` — draft only (409 `VERSION_NOT_DRAFT`); locks the lessons. */
export async function publishVersion(versionId: string): Promise<TemplateVersion> {
  const res = await apiClient.post<unknown>(`/library/versions/${versionId}/publish`);
  return parseData(templateVersionSchema, res.data);
}

/** `POST /library/versions/:vid/archive` — published only (409 `VERSION_NOT_PUBLISHED`). */
export async function archiveVersion(versionId: string): Promise<TemplateVersion> {
  const res = await apiClient.post<unknown>(`/library/versions/${versionId}/archive`);
  return parseData(templateVersionSchema, res.data);
}

/** `GET /library/versions/:vid/lessons` — ordered by position. */
export async function listLessons(versionId: string): Promise<TemplateLesson[]> {
  const res = await apiClient.get<unknown>(`/library/versions/${versionId}/lessons`);
  return parseArray(templateLessonSchema, res.data);
}

/** `POST /library/versions/:vid/lessons` — appends; a locked version answers 409 `VERSION_LOCKED`. */
export async function createLesson(versionId: string, input: LessonInput): Promise<TemplateLesson> {
  const res = await apiClient.post<unknown>(`/library/versions/${versionId}/lessons`, input);
  return parseData(templateLessonSchema, res.data);
}

/** `PUT /library/versions/:vid/lessons/order` — the complete order, every lesson id exactly once. */
export async function reorderLessons(
  versionId: string,
  lessonIds: string[],
): Promise<TemplateLesson[]> {
  const res = await apiClient.put<unknown>(`/library/versions/${versionId}/lessons/order`, {
    lesson_ids: lessonIds,
  });
  return parseArray(templateLessonSchema, res.data);
}

export async function getLesson(id: string): Promise<TemplateLesson> {
  const res = await apiClient.get<unknown>(`/library/lessons/${id}`);
  return parseData(templateLessonSchema, res.data);
}

export async function updateLesson(id: string, input: LessonInput): Promise<TemplateLesson> {
  const res = await apiClient.put<unknown>(`/library/lessons/${id}`, input);
  return parseData(templateLessonSchema, res.data);
}

/** `DELETE /library/lessons/:lid` — the remaining lessons renumber server-side. */
export async function deleteLesson(id: string): Promise<void> {
  await apiClient.delete(`/library/lessons/${id}`);
}
