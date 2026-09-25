import { apiClient } from "@/lib/api/client";
import { parseArray, parseData, parseList, type Paginated } from "@/lib/api/envelope";

import {
  exerciseGroupSchema,
  exerciseSchema,
  lessonExerciseSchema,
  lessonMaterialSchema,
  logFieldSchema,
  materialSchema,
  programTemplateSchema,
  scoreSetGroupSchema,
  templateLessonDetailSchema,
  templateLessonSchema,
  templateVersionSchema,
  versionDetailSchema,
  type Exercise,
  type ExerciseGroup,
  type ExerciseGroupInput,
  type ExerciseInput,
  type LessonExercise,
  type LessonExerciseInput,
  type LessonInput,
  type LessonMaterial,
  type LessonMaterialInput,
  type LogField,
  type LogFieldInput,
  type Material,
  type MaterialInput,
  type ProgramTemplate,
  type ScoreSetGroup,
  type ScoreSetGroupInput,
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
  /** Only templates with an open draft version. */
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

/** `POST /library/lessons/:lid/duplicate` — inserts a copy right after the source lesson. */
export async function duplicateLesson(id: string): Promise<TemplateLesson> {
  const res = await apiClient.post<unknown>(`/library/lessons/${id}/duplicate`);
  return parseData(templateLessonSchema, res.data);
}

/** `DELETE /library/versions/:vid/lessons` — clears every lesson of a draft version at once. */
export async function clearLessons(versionId: string): Promise<void> {
  await apiClient.delete(`/library/versions/${versionId}/lessons`);
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

/** `PUT /library/versions/:vid/score-set` — wholesale replace; a repeated key (within or across groups) is a 422. */
export async function setScoreSet(
  versionId: string,
  items: ScoreSetGroupInput[],
): Promise<ScoreSetGroup[]> {
  const res = await apiClient.put<unknown>(`/library/versions/${versionId}/score-set`, items);
  return parseArray(scoreSetGroupSchema, res.data);
}

/** `GET /library/versions/:vid/exercise-groups` — ordered by position. */
export async function listExerciseGroups(versionId: string): Promise<ExerciseGroup[]> {
  const res = await apiClient.get<unknown>(`/library/versions/${versionId}/exercise-groups`);
  return parseArray(exerciseGroupSchema, res.data);
}

/** `POST /library/versions/:vid/exercise-groups` — appends; a locked version answers 409 `VERSION_LOCKED`. */
export async function createExerciseGroup(
  versionId: string,
  input: ExerciseGroupInput,
): Promise<ExerciseGroup> {
  const res = await apiClient.post<unknown>(
    `/library/versions/${versionId}/exercise-groups`,
    input,
  );
  return parseData(exerciseGroupSchema, res.data);
}

/**
 * `DELETE /library/versions/:vid/exercise-groups/:gid` — the group's own
 * exercises stay attached to the lesson, only ungrouped (`group_id` → null).
 */
export async function deleteExerciseGroup(versionId: string, groupId: string): Promise<void> {
  await apiClient.delete(`/library/versions/${versionId}/exercise-groups/${groupId}`);
}

export interface ListItemsParams {
  /** Substring of the title; the API escapes wildcards. */
  q?: string;
  page?: number;
  per_page?: number;
  /** `title` (default) | `created_at`, `-` prefix for descending. */
  sort?: string;
  /** Filter by catalog status; omitted returns every status. */
  active?: boolean;
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

/** `PATCH /library/materials/:id/status` — toggles catalog visibility without touching existing links. */
export async function setMaterialStatus(id: string, active: boolean): Promise<Material> {
  const res = await apiClient.patch<unknown>(`/library/materials/${id}/status`, { active });
  return parseData(materialSchema, res.data);
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

/** `PATCH /library/exercises/:id/status` — toggles catalog visibility without touching existing links. */
export async function setExerciseStatus(id: string, active: boolean): Promise<Exercise> {
  const res = await apiClient.patch<unknown>(`/library/exercises/${id}/status`, { active });
  return parseData(exerciseSchema, res.data);
}
