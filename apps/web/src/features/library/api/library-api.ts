import { apiClient } from "@/lib/api/client";
import { parseArray, parseData, parseList, type Paginated } from "@/lib/api/envelope";

import {
  assigneeSchema,
  exerciseSchema,
  prepBoardSchema,
  lessonExerciseSchema,
  lessonMaterialSchema,
  logFieldSchema,
  materialSchema,
  programTemplateSchema,
  scoreComponentSchema,
  templateLessonDetailSchema,
  templateLessonSchema,
  templateVersionSchema,
  versionDetailSchema,
  type Exercise,
  type ExerciseInput,
  type LessonExercise,
  type LessonExerciseInput,
  type LessonInput,
  type LessonMaterial,
  type LessonMaterialInput,
  type Assignee,
  type LogField,
  type LogFieldInput,
  type Material,
  type MaterialInput,
  type AssignmentInput,
  type PrepBoard,
  type PrepInput,
  type ProgramTemplate,
  type ScoreComponent,
  type ScoreComponentInput,
  type TemplateInput,
  type TemplateLesson,
  type TemplateLessonDetail,
  type TemplateVersion,
  type VersionDetail,
} from "../schemas/library-schemas";

export interface ListTemplatesParams {
  /** Substring of code or name; the API escapes wildcards. */
  q?: string;
  page?: number;
  per_page?: number;
  /** `name` (default) | `code` | `created_at`, `-` prefix for descending. */
  sort?: string;
  /** Only templates with an open draft (the ones with a preparation board). */
  has_draft?: boolean;
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

/** `GET /library/lessons/:lid` — the lesson with its attached materials and exercises. */
export async function getLesson(id: string): Promise<TemplateLessonDetail> {
  const res = await apiClient.get<unknown>(`/library/lessons/${id}`);
  return parseData(templateLessonDetailSchema, res.data);
}

export async function updateLesson(id: string, input: LessonInput): Promise<TemplateLessonDetail> {
  const res = await apiClient.put<unknown>(`/library/lessons/${id}`, input);
  return parseData(templateLessonDetailSchema, res.data);
}

/** `DELETE /library/lessons/:lid` — the remaining lessons renumber server-side. */
export async function deleteLesson(id: string): Promise<void> {
  await apiClient.delete(`/library/lessons/${id}`);
}

/** `GET /library/versions/:vid/board` — the preparation board; readable for every version status. */
export async function getBoard(versionId: string): Promise<PrepBoard> {
  const res = await apiClient.get<unknown>(`/library/versions/${versionId}/board`);
  return parseData(prepBoardSchema, res.data);
}

/** `PATCH /library/lessons/:lid/prep` (`library.edit`) — 409 `VERSION_LOCKED` once the version is published. */
export async function updateLessonPrep(id: string, input: PrepInput): Promise<TemplateLesson> {
  const res = await apiClient.patch<unknown>(`/library/lessons/${id}/prep`, input);
  return parseData(templateLessonSchema, res.data);
}

/** `PATCH /library/lessons/:lid/assignment` (`prep.assign`) — replaces assignee and due date together. */
export async function updateLessonAssignment(
  id: string,
  input: AssignmentInput,
): Promise<TemplateLesson> {
  const res = await apiClient.patch<unknown>(`/library/lessons/${id}/assignment`, input);
  return parseData(templateLessonSchema, res.data);
}

/**
 * `GET /library/assignees` (`prep.assign`) — the center's live members
 * eligible for assignment. Unlike the member directory, this does not need
 * `members.list`.
 */
export async function listAssignees(): Promise<Assignee[]> {
  const res = await apiClient.get<unknown>("/library/assignees");
  return parseArray(assigneeSchema, res.data);
}

/**
 * `PUT /library/lessons/:lid/materials` — replaces the lesson's attachments
 * wholesale; body order becomes the display order. An unknown material id
 * is a 422 keyed `<index>.material_id`; a locked version answers 409.
 */
export async function setLessonMaterials(
  lessonId: string,
  items: LessonMaterialInput[],
): Promise<LessonMaterial[]> {
  const res = await apiClient.put<unknown>(`/library/lessons/${lessonId}/materials`, items);
  return parseArray(lessonMaterialSchema, res.data);
}

export async function setLessonExercises(
  lessonId: string,
  items: LessonExerciseInput[],
): Promise<LessonExercise[]> {
  const res = await apiClient.put<unknown>(`/library/lessons/${lessonId}/exercises`, items);
  return parseArray(lessonExerciseSchema, res.data);
}

/** `GET /library/versions/:vid` — the version with its score set, log fields and lesson details. */
export async function getVersion(versionId: string): Promise<VersionDetail> {
  const res = await apiClient.get<unknown>(`/library/versions/${versionId}`);
  return parseData(versionDetailSchema, res.data);
}

/** `PUT /library/versions/:vid/log-fields` — wholesale replace, draft only (409 `VERSION_LOCKED`). */
export async function setLogFields(versionId: string, items: LogFieldInput[]): Promise<LogField[]> {
  const res = await apiClient.put<unknown>(`/library/versions/${versionId}/log-fields`, items);
  return parseArray(logFieldSchema, res.data);
}

/** `PUT /library/versions/:vid/score-set` — wholesale replace; a repeated key is a 422 on `<index>.key`. */
export async function setScoreSet(
  versionId: string,
  items: ScoreComponentInput[],
): Promise<ScoreComponent[]> {
  const res = await apiClient.put<unknown>(`/library/versions/${versionId}/score-set`, items);
  return parseArray(scoreComponentSchema, res.data);
}

export interface ListItemsParams {
  /** Substring of the title; the API escapes wildcards. */
  q?: string;
  page?: number;
  per_page?: number;
  /** `title` (default) | `created_at`, `-` prefix for descending. */
  sort?: string;
}

/** `GET /library/materials` — the center's material catalog, paginated. */
export async function listMaterials(params: ListItemsParams = {}): Promise<Paginated<Material>> {
  const res = await apiClient.get<unknown>("/library/materials", { params });
  return parseList(materialSchema, res.data);
}

export async function createMaterial(input: MaterialInput): Promise<Material> {
  const res = await apiClient.post<unknown>("/library/materials", input);
  return parseData(materialSchema, res.data);
}

export async function updateMaterial(id: string, input: MaterialInput): Promise<Material> {
  const res = await apiClient.put<unknown>(`/library/materials/${id}`, input);
  return parseData(materialSchema, res.data);
}

/** `DELETE /library/materials/:id` — refused with 409 `MATERIAL_IN_USE` while any lesson links it. */
export async function deleteMaterial(id: string): Promise<void> {
  await apiClient.delete(`/library/materials/${id}`);
}

export async function listExercises(params: ListItemsParams = {}): Promise<Paginated<Exercise>> {
  const res = await apiClient.get<unknown>("/library/exercises", { params });
  return parseList(exerciseSchema, res.data);
}

export async function createExercise(input: ExerciseInput): Promise<Exercise> {
  const res = await apiClient.post<unknown>("/library/exercises", input);
  return parseData(exerciseSchema, res.data);
}

export async function updateExercise(id: string, input: ExerciseInput): Promise<Exercise> {
  const res = await apiClient.put<unknown>(`/library/exercises/${id}`, input);
  return parseData(exerciseSchema, res.data);
}

/** `DELETE /library/exercises/:id` — refused with 409 `EXERCISE_IN_USE` while any lesson links it. */
export async function deleteExercise(id: string): Promise<void> {
  await apiClient.delete(`/library/exercises/${id}`);
}
