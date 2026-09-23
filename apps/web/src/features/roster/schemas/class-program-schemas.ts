import { z } from "zod";

import { templateLessonDetailSchema } from "@/features/library";

/**
 * `classprogram.ProgramResponse` — the template version a class follows.
 * `GET /classes/:id/program` answers `data: null` while nothing is applied.
 */
export const classProgramSchema = z.object({
  template_version_id: z.string(),
  template_id: z.string(),
  template_name: z.string(),
  version_no: z.number().int(),
  /** The library's current status of that version; "archived" means the class should move on. */
  version_status: z.enum(["draft", "published", "archived"]),
  lesson_count: z.number().int(),
  applied_at: z.string(),
  applied_by: z.string(),
});
export type ClassProgram = z.infer<typeof classProgramSchema>;

/**
 * Template lessons read through the class (`GET /classes/:id/program/lessons`):
 * the same rows the library's lesson detail returns, materials and exercises
 * included, so the tabs never need to know which version they came from.
 */
export const classLessonSchema = templateLessonDetailSchema;
export type ClassLesson = z.infer<typeof classLessonSchema>;

/** `PUT /classes/:id/program` body; `confirm` overrides a 409 `CURRICULUM_DIFFERS`. */
export interface ApplyClassProgramInput {
  template_version_id: string;
  confirm?: boolean;
}

/** `classchat.MessageResponse`. */
export const classMessageSchema = z.object({
  id: z.string(),
  class_id: z.string(),
  author_id: z.string(),
  author_name: z.string(),
  body: z.string(),
  created_at: z.string(),
});
export type ClassMessage = z.infer<typeof classMessageSchema>;

/** Newest first; `next_cursor` is the id to pass as `before`, empty on the last page. */
export const classMessagePageSchema = z.object({
  items: z.array(classMessageSchema),
  next_cursor: z.string(),
});
export type ClassMessagePage = z.infer<typeof classMessagePageSchema>;

export const CLASS_MESSAGE_MAX = 2000;

/**
 * The slice of `courses.CourseResponse` the program card needs: the class's
 * embedded course ref carries no default version, so "Áp dụng từ khóa mẫu"
 * reads the course itself. Kept here rather than importing the courses
 * feature, which already depends on roster.
 */
export const courseDefaultTemplateSchema = z.object({
  id: z.string(),
  default_template_version_id: z.string().nullable(),
});
export type CourseDefaultTemplate = z.infer<typeof courseDefaultTemplateSchema>;
