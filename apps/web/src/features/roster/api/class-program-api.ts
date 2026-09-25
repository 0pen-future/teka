import { apiClient } from "@/lib/api/client";
import { parseArray, parseData } from "@/lib/api/envelope";

import {
  classLessonSchema,
  classProgramSchema,
  courseDefaultTemplateSchema,
  type ApplyClassProgramInput,
  type ClassLesson,
  type ClassProgram,
  type CourseDefaultTemplate,
} from "../schemas/class-program-schemas";

/**
 * `GET /classes/:id/program` — null while the class follows no template. The
 * API envelope omits `data` entirely for a nil payload, so the schema must
 * accept a missing key as well as an explicit null.
 */
export async function getClassProgram(classId: string): Promise<ClassProgram | null> {
  const res = await apiClient.get<unknown>(`/classes/${classId}/program`);
  return parseData(classProgramSchema.nullish(), res.data) ?? null;
}

/** `GET /classes/:id/program/lessons` — `[]` while no program is applied. */
export async function listClassProgramLessons(classId: string): Promise<ClassLesson[]> {
  const res = await apiClient.get<unknown>(`/classes/${classId}/program/lessons`);
  return parseArray(classLessonSchema, res.data);
}

/**
 * `PUT /classes/:id/program` — applies or swaps the version and rewrites the
 * class curriculum's lesson titles. Answers 409 `CURRICULUM_DIFFERS` (fields
 * `current_count`/`template_count`) when the class already keeps a different
 * lesson list and `confirm` is not set.
 */
export async function applyClassProgram(
  classId: string,
  input: ApplyClassProgramInput,
): Promise<ClassProgram> {
  const res = await apiClient.put<unknown>(`/classes/${classId}/program`, input);
  return parseData(classProgramSchema, res.data);
}

/** `DELETE /classes/:id/program` — drops the link only; curriculum and lesson plans stay. */
export async function removeClassProgram(classId: string): Promise<void> {
  await apiClient.delete(`/classes/${classId}/program`);
}

/** `GET /courses/:id`, read only for the course's default template version. */
export async function getCourseDefaultTemplate(courseId: string): Promise<CourseDefaultTemplate> {
  const res = await apiClient.get<unknown>(`/courses/${courseId}`);
  return parseData(courseDefaultTemplateSchema, res.data);
}
