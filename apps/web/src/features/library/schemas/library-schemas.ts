import { z } from "zod";

/**
 * Wire shapes of `apps/api/internal/features/library/dto.go`. Optional text
 * columns are `*string` server-side and serialize as `null`, never omitted,
 * hence `.nullable()` throughout.
 */
export const templateVersionStatusSchema = z.enum(["draft", "published", "archived"]);
export type TemplateVersionStatus = z.infer<typeof templateVersionStatusSchema>;

/** Preparation states of a draft lesson, in board order (`library.PrepTodo` …). */
export const PREP_STATUSES = ["todo", "doing", "review", "done"] as const;
export const prepStatusSchema = z.enum(PREP_STATUSES);
export type PrepStatus = z.infer<typeof prepStatusSchema>;

export const checklistItemSchema = z.object({
  label: z.string(),
  done: z.boolean(),
});
export type ChecklistItem = z.infer<typeof checklistItemSchema>;

/** `library.PrepSummaryResponse` — present only while the template has an open draft. */
export const prepSummarySchema = z.object({
  lesson_count: z.number().int(),
  done_count: z.number().int(),
  assignees: z.array(z.string()),
});
export type PrepSummary = z.infer<typeof prepSummarySchema>;

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
  draft_version_id: z.string().nullable(),
  version_count: z.number().int(),
  prep: prepSummarySchema.nullable(),
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
  prep_status: prepStatusSchema,
  assignee_id: z.string().nullable(),
  /** Calendar day `YYYY-MM-DD`, no time part. */
  due_date: z.string().nullable(),
  checklist: z.array(checklistItemSchema),
  created_at: z.string(),
  updated_at: z.string(),
});
export type TemplateLesson = z.infer<typeof templateLessonSchema>;

/** `library.BoardCardResponse` — one lesson on the preparation board. */
export const boardCardSchema = z.object({
  id: z.string(),
  position: z.number().int(),
  title: z.string(),
  prep_status: prepStatusSchema,
  assignee_id: z.string().nullable(),
  assignee_name: z.string().nullable(),
  due_date: z.string().nullable(),
  checklist_done: z.number().int(),
  checklist_total: z.number().int(),
});
export type BoardCard = z.infer<typeof boardCardSchema>;

export const boardColumnSchema = z.object({
  status: prepStatusSchema,
  lessons: z.array(boardCardSchema),
});
export type BoardColumn = z.infer<typeof boardColumnSchema>;

/** `GET /library/versions/:vid/board` — the four fixed status columns of one version. */
export const prepBoardSchema = z.object({
  template: programTemplateSchema,
  version: templateVersionSchema,
  columns: z.array(boardColumnSchema),
});
export type PrepBoard = z.infer<typeof prepBoardSchema>;

/** `library.TemplateRequest` — create and full-replace share one body. */
export interface TemplateInput {
  code: string;
  name: string;
  subject: string | null;
  level: string | null;
  description: string | null;
  /** Create only: seeds "Buổi 1..N" into the opening draft (1–100). */
  lesson_count?: number;
}

/** `library.PrepRequest` — an omitted field keeps its value; a checklist replaces the stored one. */
export interface PrepInput {
  prep_status?: PrepStatus;
  checklist?: ChecklistItem[];
}

/** `library.AssignmentRequest` — a full replace: `null` clears the field. */
export interface AssignmentInput {
  assignee_id: string | null;
  due_date: string | null;
}

/**
 * `library.AssigneeResponse` — one live center member the assign picker can
 * offer. Deliberately not the member directory (`members.list`): `prep.assign`
 * alone must be enough to see who can be assigned.
 */
export const assigneeSchema = z.object({
  id: z.string(),
  full_name: z.string(),
});
export type Assignee = z.infer<typeof assigneeSchema>;

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

// --- Materials, exercises and what a version carries besides lessons ---

export const materialKindSchema = z.enum(["link", "doc", "video", "other"]);
export type MaterialKind = z.infer<typeof materialKindSchema>;

export const materialSchema = z.object({
  id: z.string(),
  title: z.string(),
  kind: materialKindSchema,
  url: z.string().nullable(),
  description: z.string().nullable(),
  tags: z.array(z.string()),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Material = z.infer<typeof materialSchema>;

export const exerciseSchema = z.object({
  id: z.string(),
  title: z.string(),
  description: z.string().nullable(),
  difficulty: z.number().int().nullable(),
  tags: z.array(z.string()),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Exercise = z.infer<typeof exerciseSchema>;

/** A material as attached to one lesson: the catalog row plus the link's own columns. */
export const lessonMaterialSchema = materialSchema.extend({
  shared_with_students: z.boolean(),
  position: z.number().int(),
});
export type LessonMaterial = z.infer<typeof lessonMaterialSchema>;

export const lessonExerciseSchema = exerciseSchema.extend({
  position: z.number().int(),
});
export type LessonExercise = z.infer<typeof lessonExerciseSchema>;

/** `library.LessonDetailResponse` — what `GET/PUT /library/lessons/:lid` answer. */
export const templateLessonDetailSchema = templateLessonSchema.extend({
  materials: z.array(lessonMaterialSchema),
  exercises: z.array(lessonExerciseSchema),
});
export type TemplateLessonDetail = z.infer<typeof templateLessonDetailSchema>;

export const logFieldKindSchema = z.enum(["text", "number", "select", "checkbox"]);
export type LogFieldKind = z.infer<typeof logFieldKindSchema>;

export const logFieldSchema = z.object({
  id: z.string(),
  position: z.number().int(),
  label: z.string(),
  kind: logFieldKindSchema,
  options: z.array(z.string()),
  required: z.boolean(),
});
export type LogField = z.infer<typeof logFieldSchema>;

export const scoreComponentSchema = z.object({
  key: z.string(),
  label: z.string(),
  max: z.number(),
  weight: z.number(),
});
export type ScoreComponent = z.infer<typeof scoreComponentSchema>;

/** `library.VersionDetailResponse` — a version with its score set, log fields and lessons. */
export const versionDetailSchema = templateVersionSchema.extend({
  score_set: z.array(scoreComponentSchema),
  log_fields: z.array(logFieldSchema),
  lessons: z.array(templateLessonDetailSchema),
});
export type VersionDetail = z.infer<typeof versionDetailSchema>;

/** `library.MaterialRequest` — create and full-replace share one body. */
export interface MaterialInput {
  title: string;
  kind: MaterialKind;
  url: string | null;
  description: string | null;
  tags: string[];
}

/** `library.ExerciseRequest`. */
export interface ExerciseInput {
  title: string;
  description: string | null;
  difficulty: number | null;
  tags: string[];
}

/** One entry of the wholesale-replace body of `PUT /library/lessons/:lid/materials`; body order is display order. */
export interface LessonMaterialInput {
  material_id: string;
  shared_with_students: boolean;
}

export interface LessonExerciseInput {
  exercise_id: string;
}

/** One entry of `PUT /library/versions/:vid/log-fields`; options only matter for kind `select`. */
export interface LogFieldInput {
  label: string;
  kind: LogFieldKind;
  options: string[];
  required: boolean;
}

/** One entry of `PUT /library/versions/:vid/score-set`; the key is what a class's grading refers to. */
export interface ScoreComponentInput {
  key: string;
  label: string;
  max: number;
  weight: number;
}

/** "a, b ,,c" → ["a", "b", "c"]; the server trims and drops blanks the same way. */
export function splitTags(text: string): string[] {
  return text
    .split(",")
    .map((tag) => tag.trim())
    .filter((tag) => tag !== "");
}

const tagsText = z
  .string()
  .trim()
  .refine((text) => splitTags(text).length <= 20, "Tối đa 20 thẻ")
  .refine((text) => splitTags(text).every((tag) => tag.length <= 50), "Mỗi thẻ tối đa 50 ký tự");

/** Material dialog state; tags travel as one comma-separated field. */
/** Mirrors the API: an empty link is fine, a given one must be absolute http(s). */
const optionalHttpUrl = optionalText(2000).refine(
  (text) => text === "" || /^https?:\/\/[^/\s]+/i.test(text),
  "Đường dẫn phải bắt đầu bằng http:// hoặc https://",
);

export const materialFormSchema = z.object({
  title: z.string().trim().min(1, "Bắt buộc nhập tên học liệu").max(200, "Tối đa 200 ký tự"),
  kind: materialKindSchema,
  url: optionalHttpUrl,
  description: optionalText(4000),
  tags: tagsText,
});
export type MaterialFormInput = z.input<typeof materialFormSchema>;
export type MaterialFormValues = z.output<typeof materialFormSchema>;

export function toMaterialInput(values: MaterialFormValues): MaterialInput {
  return {
    title: values.title,
    kind: values.kind,
    url: blankToNull(values.url),
    description: blankToNull(values.description),
    tags: splitTags(values.tags),
  };
}

export function toMaterialForm(material: Material): MaterialFormInput {
  return {
    title: material.title,
    kind: material.kind,
    url: material.url ?? "",
    description: material.description ?? "",
    tags: material.tags.join(", "),
  };
}

/** Exercise dialog state; difficulty is a select whose empty value means "not set". */
export const exerciseFormSchema = z.object({
  title: z.string().trim().min(1, "Bắt buộc nhập tên bài tập").max(200, "Tối đa 200 ký tự"),
  description: optionalText(4000),
  difficulty: z.enum(["", "1", "2", "3", "4", "5"]),
  tags: tagsText,
});
export type ExerciseFormInput = z.input<typeof exerciseFormSchema>;
export type ExerciseFormValues = z.output<typeof exerciseFormSchema>;

export function toExerciseInput(values: ExerciseFormValues): ExerciseInput {
  return {
    title: values.title,
    description: blankToNull(values.description),
    difficulty: values.difficulty === "" ? null : Number(values.difficulty),
    tags: splitTags(values.tags),
  };
}

export function toExerciseForm(exercise: Exercise): ExerciseFormInput {
  return {
    title: exercise.title,
    description: exercise.description ?? "",
    difficulty:
      exercise.difficulty === null
        ? ""
        : (String(exercise.difficulty) as ExerciseFormInput["difficulty"]),
    tags: exercise.tags.join(", "),
  };
}
