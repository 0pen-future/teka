import { z } from "zod";

/**
 * Wire shapes of `apps/api/internal/features/library/dto.go`. Optional text
 * columns are `*string` server-side and serialize as `null`, never omitted,
 * hence `.nullable()` throughout.
 */
export const templateVersionStatusSchema = z.enum(["draft", "published", "archived"]);
export type TemplateVersionStatus = z.infer<typeof templateVersionStatusSchema>;

export const programTemplateSchema = z.object({
  id: z.string(),
  code: z.string(),
  name: z.string(),
  subject: z.string().nullable(),
  level: z.string().nullable(),
  description: z.string().nullable(),
  created_by: z.string().nullable(),
  published_version_no: z.number().int().nullable(),
  draft_version_no: z.number().int().nullable(),
  version_count: z.number().int(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type ProgramTemplate = z.infer<typeof programTemplateSchema>;

export const templateVersionSchema = z.object({
  id: z.string(),
  template_id: z.string(),
  version_no: z.number().int(),
  status: templateVersionStatusSchema,
  changelog: z.string().nullable(),
  published_at: z.string().nullable(),
  created_by: z.string().nullable(),
  lesson_count: z.number().int(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type TemplateVersion = z.infer<typeof templateVersionSchema>;

export const templateLessonSchema = z.object({
  id: z.string(),
  version_id: z.string(),
  position: z.number().int(),
  title: z.string(),
  objectives: z.string().nullable(),
  duration_min: z.number().int().nullable(),
  homework_note: z.string().nullable(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type TemplateLesson = z.infer<typeof templateLessonSchema>;

/** `library.TemplateRequest` — create and full-replace share one body. */
export interface TemplateInput {
  code: string;
  name: string;
  subject: string | null;
  level: string | null;
  description: string | null;
}

/** `library.LessonRequest` — position is never in the body (append on create, reorder endpoint otherwise). */
export interface LessonInput {
  title: string;
  objectives: string | null;
  duration_min: number | null;
  homework_note: string | null;
}

/** Stricter than `classcode.Valid` (`apps/api/internal/shared/classcode`): the server also allows a leading `-`; both upper-case before checking. */
const templateCodePattern = /^[A-Z0-9][A-Z0-9-]*$/;

const optionalText = (max: number) => z.string().trim().max(max, `Tối đa ${max} ký tự`);

/** Form state for the template dialog; empty optional fields travel as `null` (see `toTemplateInput`). */
export const templateFormSchema = z.object({
  code: z
    .string()
    .trim()
    .transform((value) => value.toUpperCase())
    .pipe(
      z
        .string()
        .min(2, "Mã từ 2 đến 20 ký tự")
        .max(20, "Mã từ 2 đến 20 ký tự")
        .regex(templateCodePattern, "Chỉ dùng chữ, số và dấu gạch ngang"),
    ),
  name: z.string().trim().min(1, "Bắt buộc nhập tên").max(200, "Tối đa 200 ký tự"),
  subject: optionalText(100),
  level: optionalText(100),
  description: optionalText(2000),
});
export type TemplateFormInput = z.input<typeof templateFormSchema>;
export type TemplateFormValues = z.output<typeof templateFormSchema>;

function blankToNull(value: string): string | null {
  return value === "" ? null : value;
}

export function toTemplateInput(values: TemplateFormValues): TemplateInput {
  return {
    code: values.code,
    name: values.name,
    subject: blankToNull(values.subject),
    level: blankToNull(values.level),
    description: blankToNull(values.description),
  };
}

export function toTemplateForm(template: ProgramTemplate): TemplateFormInput {
  return {
    code: template.code,
    name: template.name,
    subject: template.subject ?? "",
    level: template.level ?? "",
    description: template.description ?? "",
  };
}

/**
 * Lesson form: the duration input is a free text field so an empty value can
 * mean "not set"; it must parse to a whole number of minutes within a day
 * (`binding:"min=1,max=1440"` server-side).
 */
export const lessonFormSchema = z.object({
  title: z.string().trim().min(1, "Bắt buộc nhập tên buổi").max(200, "Tối đa 200 ký tự"),
  objectives: optionalText(4000),
  duration_min: z
    .string()
    .trim()
    .refine((value) => value === "" || /^\d+$/.test(value), "Nhập số phút nguyên")
    .refine((value) => {
      if (value === "") return true;
      const minutes = Number(value);
      return minutes >= 1 && minutes <= 1440;
    }, "Thời lượng từ 1 đến 1440 phút"),
  homework_note: optionalText(4000),
});
export type LessonFormInput = z.input<typeof lessonFormSchema>;
export type LessonFormValues = z.output<typeof lessonFormSchema>;

export function toLessonInput(values: LessonFormValues): LessonInput {
  return {
    title: values.title,
    objectives: blankToNull(values.objectives),
    duration_min: values.duration_min === "" ? null : Number(values.duration_min),
    homework_note: blankToNull(values.homework_note),
  };
}

export function toLessonForm(lesson: TemplateLesson): LessonFormInput {
  return {
    title: lesson.title,
    objectives: lesson.objectives ?? "",
    duration_min: lesson.duration_min === null ? "" : String(lesson.duration_min),
    homework_note: lesson.homework_note ?? "",
  };
}
