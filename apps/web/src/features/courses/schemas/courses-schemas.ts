import { z } from "zod";

/**
 * Wire shapes of `apps/api/internal/features/courses/dto.go`. Optional
 * columns are pointers server-side and serialize as `null`, never omitted.
 */
export const courseStatusSchema = z.enum(["draft", "active", "archived"]);
export type CourseStatus = z.infer<typeof courseStatusSchema>;

export const tuitionPackSchema = z.object({
  id: z.string(),
  name: z.string(),
  sessions: z.number().int(),
  price: z.number().int(),
  position: z.number().int(),
});
export type TuitionPack = z.infer<typeof tuitionPackSchema>;

export const defaultTemplateSchema = z.object({
  version_id: z.string(),
  version_no: z.number().int(),
  // The version's current status: published when chosen, possibly archived since.
  status: z.enum(["draft", "published", "archived"]),
  template_id: z.string(),
  code: z.string(),
  name: z.string(),
});
export type DefaultTemplate = z.infer<typeof defaultTemplateSchema>;

export const courseSchema = z.object({
  id: z.string(),
  code: z.string(),
  name: z.string(),
  subject: z.string().nullable(),
  level: z.string().nullable(),
  description: z.string().nullable(),
  status: courseStatusSchema,
  default_template_version_id: z.string().nullable(),
  default_template: defaultTemplateSchema.nullable(),
  default_unit_price: z.number().int(),
  total_sessions: z.number().int().nullable(),
  duration_min: z.number().int().nullable(),
  classes_running: z.number().int(),
  classes_upcoming: z.number().int(),
  tuition_packs: z.array(tuitionPackSchema),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Course = z.infer<typeof courseSchema>;

/** `courses.CourseRequest` — create and full-replace share one body. */
export interface CourseInput {
  code: string;
  name: string;
  subject: string | null;
  level: string | null;
  description: string | null;
  status: CourseStatus;
  default_template_version_id: string | null;
  default_unit_price: number;
  total_sessions: number | null;
  duration_min: number | null;
}

/** `courses.TuitionPackInput` — the wholesale-replaced pack list. */
export interface TuitionPackInput {
  name: string;
  sessions: number;
  price: number;
}

/** Shared by course and learning path codes: upper-case letters, digits and dashes. */
export const courseCodePattern = /^[A-Z0-9][A-Z0-9-]*$/;

const optionalText = (max: number) => z.string().trim().max(max, `Tối đa ${max} ký tự`);

/**
 * A blank optional whole-number input means "not set"; otherwise it must be
 * an integer inside the API's bounds.
 */
const optionalWhole = (min: number, max: number, message: string) =>
  z
    .string()
    .trim()
    .refine((value) => value === "" || (/^\d+$/.test(value) && Number(value) >= min), message)
    .refine((value) => value === "" || Number(value) <= max, message)
    .transform((value) => (value === "" ? null : Number(value)));

/**
 * Form state for the course dialog; empty optional fields travel as `null`
 * and the default template is carried outside the form (see `toCourseInput`).
 */
export const courseFormSchema = z.object({
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
  subject: optionalText(100),
  level: optionalText(100),
  description: optionalText(2000),
  status: courseStatusSchema,
  default_unit_price: z
    .string()
    .trim()
    .regex(/^\d+$/, "Đơn giá phải là số nguyên không âm")
    .transform(Number),
  total_sessions: optionalWhole(1, 1000, "Số buổi từ 1 đến 1000"),
  duration_min: optionalWhole(1, 1440, "Thời lượng từ 1 đến 1440 phút"),
});
export type CourseFormInput = z.input<typeof courseFormSchema>;
export type CourseFormValues = z.output<typeof courseFormSchema>;

function blankToNull(value: string): string | null {
  return value === "" ? null : value;
}

export function toCourseInput(
  values: CourseFormValues,
  defaultTemplateVersionId: string | null,
): CourseInput {
  return {
    code: values.code,
    name: values.name,
    subject: blankToNull(values.subject),
    level: blankToNull(values.level),
    description: blankToNull(values.description),
    status: values.status,
    default_template_version_id: defaultTemplateVersionId,
    default_unit_price: values.default_unit_price,
    total_sessions: values.total_sessions,
    duration_min: values.duration_min,
  };
}

/** The full-replace body for a stored course, so a one-field change resends the rest unchanged. */
export function courseToInput(course: Course): CourseInput {
  return {
    code: course.code,
    name: course.name,
    subject: course.subject,
    level: course.level,
    description: course.description,
    status: course.status,
    default_template_version_id: course.default_template_version_id,
    default_unit_price: course.default_unit_price,
    total_sessions: course.total_sessions,
    duration_min: course.duration_min,
  };
}

export function toCourseForm(course: Course): CourseFormInput {
  return {
    code: course.code,
    name: course.name,
    subject: course.subject ?? "",
    level: course.level ?? "",
    description: course.description ?? "",
    status: course.status,
    default_unit_price: String(course.default_unit_price),
    total_sessions: course.total_sessions === null ? "" : String(course.total_sessions),
    duration_min: course.duration_min === null ? "" : String(course.duration_min),
  };
}
