import { z } from "zod";

import { courseCodePattern, courseStatusSchema } from "./courses-schemas";

export const pathStatusSchema = z.enum(["draft", "active", "archived"]);
export type PathStatus = z.infer<typeof pathStatusSchema>;

/** A course recommended inside a stage, as the API embeds it. */
export const stageCourseSchema = z.object({
  id: z.string(),
  code: z.string(),
  name: z.string(),
  status: courseStatusSchema,
  position: z.number().int(),
});
export type StageCourse = z.infer<typeof stageCourseSchema>;

export const stageSchema = z.object({
  id: z.string(),
  position: z.number().int(),
  name: z.string(),
  goal: z.string().nullable(),
  courses: z.array(stageCourseSchema),
});
export type Stage = z.infer<typeof stageSchema>;

export const learningPathSchema = z.object({
  id: z.string(),
  code: z.string(),
  name: z.string(),
  description: z.string().nullable(),
  status: pathStatusSchema,
  stage_count: z.number().int(),
  course_count: z.number().int(),
  stages: z.array(stageSchema),
  created_at: z.string(),
  updated_at: z.string(),
});
export type LearningPath = z.infer<typeof learningPathSchema>;

/** One row of `GET /courses/:id/paths`: a path and the stage that recommends the course. */
export const coursePathSchema = z.object({
  id: z.string(),
  code: z.string(),
  name: z.string(),
  status: pathStatusSchema,
  stage_id: z.string(),
  stage_name: z.string(),
  stage_position: z.number().int(),
});
export type CoursePath = z.infer<typeof coursePathSchema>;

export interface PathInput {
  code: string;
  name: string;
  description: string | null;
  status: PathStatus;
}

export interface StageInput {
  name: string;
  goal: string | null;
}

// --- Forms ---

function optionalText(max: number) {
  return z.string().trim().max(max, `Tối đa ${max} ký tự`);
}

/** Path codes follow the course code rule: upper-case letters, digits and dashes. */
export const pathFormSchema = z.object({
  code: z
    .string()
    .trim()
    .transform((value) => value.toUpperCase())
    .pipe(
      z
        .string()
        .min(2, "Mã từ 2 đến 20 ký tự")
        .max(20, "Mã từ 2 đến 20 ký tự")
        .regex(courseCodePattern, "Chỉ dùng chữ, số và dấu gạch ngang"),
    ),
  name: z.string().trim().min(1, "Bắt buộc nhập tên").max(200, "Tối đa 200 ký tự"),
  description: optionalText(2000),
  status: pathStatusSchema,
});
export type PathFormInput = z.input<typeof pathFormSchema>;
export type PathFormValues = z.output<typeof pathFormSchema>;

export const stageFormSchema = z.object({
  name: z.string().trim().min(1, "Bắt buộc nhập tên").max(200, "Tối đa 200 ký tự"),
  goal: optionalText(2000),
});
export type StageFormInput = z.input<typeof stageFormSchema>;
export type StageFormValues = z.output<typeof stageFormSchema>;

function blankToNull(value: string): string | null {
  return value === "" ? null : value;
}

export function toPathInput(values: PathFormValues): PathInput {
  return {
    code: values.code,
    name: values.name,
    description: blankToNull(values.description),
    status: values.status,
  };
}

export function pathToForm(path: LearningPath): PathFormInput {
  return {
    code: path.code,
    name: path.name,
    description: path.description ?? "",
    status: path.status,
  };
}

export function toStageInput(values: StageFormValues): StageInput {
  return { name: values.name, goal: blankToNull(values.goal) };
}

export function stageToForm(stage: Stage): StageFormInput {
  return { name: stage.name, goal: stage.goal ?? "" };
}
