import { z } from "zod";

/**
 * Wire shapes of `apps/api/internal/features/library/dto.go`. Optional text
 * columns are `*string` server-side and serialize as `null`, never omitted,
 * hence `.nullable()` throughout.
 */
export const templateVersionStatusSchema = z.enum(["draft", "published", "archived"]);
export type TemplateVersionStatus = z.infer<typeof templateVersionStatusSchema>;

/** `library.VersionRefResponse` — one entry of a template's version history. */
export const versionRefResponseSchema = z.object({
  id: z.string(),
  version_no: z.number().int(),
  status: templateVersionStatusSchema,
});
export type VersionRefResponse = z.infer<typeof versionRefResponseSchema>;

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
  /** Classes bound to any version of the template. */
  class_count: z.number().int(),
  /** The released version's lesson count when one exists, else the open draft's. */
  lesson_count: z.number().int(),
  versions: z.array(versionRefResponseSchema),
  created_at: z.string(),
  updated_at: z.string(),
});
export type ProgramTemplate = z.infer<typeof programTemplateSchema>;

/** `library.VersionClassRefResponse` — one class bound to a version. */
export const versionClassRefResponseSchema = z.object({
  id: z.string(),
  name: z.string(),
});
export type VersionClassRefResponse = z.infer<typeof versionClassRefResponseSchema>;

export const templateVersionSchema = z.object({
  id: z.string(),
  template_id: z.string(),
  version_no: z.number().int(),
  status: templateVersionStatusSchema,
  changelog: z.string().nullable(),
  published_at: z.string().nullable(),
  created_by: z.string().nullable(),
  lesson_count: z.number().int(),
  /** True count of classes bound to this version. */
  class_count: z.number().int(),
  /** Capped, ordered by name (see `library.ListVersionClasses`). */
  classes: z.array(versionClassRefResponseSchema),
  created_at: z.string(),
  updated_at: z.string(),
});
export type TemplateVersion = z.infer<typeof templateVersionSchema>;

/** Whether a lesson happens in class at a fixed time or the student works through it alone. */
export const lessonModeSchema = z.enum(["scheduled", "self_study"]);
export type LessonMode = z.infer<typeof lessonModeSchema>;

export const templateLessonSchema = z.object({
  id: z.string(),
  version_id: z.string(),
  position: z.number().int(),
  title: z.string(),
  mode: lessonModeSchema,
  unit: z.string().nullable(),
  objectives: z.string().nullable(),
  duration_min: z.number().int().nullable(),
  homework_note: z.string().nullable(),
  /** 0 unless the row came from a list/detail response (see `lessonRowResponse`). */
  material_count: z.number().int(),
  exercise_count: z.number().int(),
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
  /** Create only: seeds "Buổi 1..N" into the opening draft (1–100). */
  lesson_count?: number;
}

/**
 * `library.LessonRequest` — position is never in the body (append on create,
 * reorder endpoint otherwise). `mode` and `unit` are optional on the wire
 * (the server defaults an omitted mode to `scheduled`), but the lesson form
 * always sends both explicitly once loaded from a lesson.
 */
export interface LessonInput {
  title: string;
  mode?: LessonMode;
  unit?: string | null;
  objectives: string | null;
  duration_min: number | null;
  homework_note: string | null;
}

/** Stricter than `classcode.Valid` (`apps/api/internal/shared/classcode`): the server also allows a leading `-`; both upper-case before checking. */
const templateCodePattern = /^[A-Z0-9][A-Z0-9-]*$/;

export const optionalText = (max: number) => z.string().trim().max(max, `Tối đa ${max} ký tự`);

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
 * mean "not set"; when given it must be a whole quarter-hour of minutes
 * within a day (the number stepper moves it in steps of 15, min 15,
 * `binding:"min=1,max=1440"` server-side).
 */
export const lessonFormSchema = z.object({
  title: z.string().trim().min(1, "Bắt buộc nhập tên buổi").max(200, "Tối đa 200 ký tự"),
  mode: lessonModeSchema,
  unit: optionalText(100),
  objectives: optionalText(4000),
  duration_min: z
    .string()
    .trim()
    .refine((value) => value === "" || /^\d+$/.test(value), "Nhập số phút nguyên")
    .refine((value) => {
      if (value === "") return true;
      const minutes = Number(value);
      return minutes >= 15 && minutes <= 1440 && minutes % 15 === 0;
    }, "Thời lượng là bội số của 15 phút, từ 15 đến 1440 phút"),
  homework_note: optionalText(4000),
});
export type LessonFormInput = z.input<typeof lessonFormSchema>;
export type LessonFormValues = z.output<typeof lessonFormSchema>;

export function toLessonInput(values: LessonFormValues): LessonInput {
  return {
    title: values.title,
    mode: values.mode,
    unit: blankToNull(values.unit),
    objectives: blankToNull(values.objectives),
    duration_min: values.duration_min === "" ? null : Number(values.duration_min),
    homework_note: blankToNull(values.homework_note),
  };
}

export function toLessonForm(lesson: TemplateLesson): LessonFormInput {
  return {
    title: lesson.title,
    mode: lesson.mode,
    unit: lesson.unit ?? "",
    objectives: lesson.objectives ?? "",
    duration_min: lesson.duration_min === null ? "" : String(lesson.duration_min),
    homework_note: lesson.homework_note ?? "",
  };
}

// --- Materials, exercises and what a version carries besides lessons ---

/** `MaterialKindOther` stays readable on legacy rows but is not offered on new input. */
export const materialKindSchema = z.enum([
  "video",
  "audio",
  "image",
  "doc",
  "note",
  "live",
  "link",
  "other",
]);
export type MaterialKind = z.infer<typeof materialKindSchema>;

export const materialSchema = z.object({
  id: z.string(),
  title: z.string(),
  kind: materialKindSchema,
  url: z.string().nullable(),
  description: z.string().nullable(),
  tags: z.array(z.string()),
  active: z.boolean(),
  /** 0 unless the row came from the bank's own Get/List/Create/Update/status response. */
  lesson_count: z.number().int(),
  template_count: z.number().int(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Material = z.infer<typeof materialSchema>;

export const exerciseSchema = z.object({
  id: z.string(),
  title: z.string(),
  description: z.string().nullable(),
  difficulty: z.number().int().nullable(),
  code: z.string(),
  skill: z.string().nullable(),
  level: z.string().nullable(),
  tags: z.array(z.string()),
  active: z.boolean(),
  /** 0 unless the row came from the bank's own Get/List/Create/Update/status response. */
  lesson_count: z.number().int(),
  template_count: z.number().int(),
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
  /** Which of the version's exercise groups this attachment belongs to, if any. */
  group_id: z.string().nullable(),
  position: z.number().int(),
});
export type LessonExercise = z.infer<typeof lessonExerciseSchema>;

/** `library.LessonDetailResponse` — what `GET/PUT /library/lessons/:lid` answer. */
export const templateLessonDetailSchema = templateLessonSchema.extend({
  materials: z.array(lessonMaterialSchema),
  exercises: z.array(lessonExerciseSchema),
});
export type TemplateLessonDetail = z.infer<typeof templateLessonDetailSchema>;

export const logFieldKindSchema = z.enum([
  "text",
  "long_text",
  "checkbox",
  "student",
  "number",
  "select",
]);
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

/** `library.ScoreSetGroup` — one named bucket of score components (e.g. "Giữa kỳ", "Cuối kỳ"). */
export const scoreSetGroupSchema = z.object({
  key: z.string(),
  title: z.string(),
  components: z.array(scoreComponentSchema),
});
export type ScoreSetGroup = z.infer<typeof scoreSetGroupSchema>;

/** `library.ExerciseGroupResponse` — a named bucket of exercises within a version. */
export const exerciseGroupSchema = z.object({
  id: z.string(),
  version_id: z.string(),
  name: z.string(),
  position: z.number().int(),
  exercise_count: z.number().int(),
});
export type ExerciseGroup = z.infer<typeof exerciseGroupSchema>;

/** `library.VersionDetailResponse` — a version with its score set, log fields and lessons. */
export const versionDetailSchema = templateVersionSchema.extend({
  score_set: z.array(scoreSetGroupSchema),
  log_fields: z.array(logFieldSchema),
  lessons: z.array(templateLessonDetailSchema),
});
export type VersionDetail = z.infer<typeof versionDetailSchema>;

/**
 * `library.MaterialRequest` — create and full-replace share one body. Active
 * is not part of it: `setMaterialStatus` toggles that separately.
 */
export interface MaterialInput {
  title: string;
  kind: MaterialKind;
  url: string | null;
  description: string | null;
  tags: string[];
}

/**
 * `library.ExerciseRequest`. `code`, `skill` and `level` are optional on the
 * wire and left unset by the exercise dialog for now, the same "do not send
 * what the form cannot yet edit" reasoning as `LessonInput.mode`/`unit`.
 * Active is not part of it: `setExerciseStatus` toggles that separately.
 */
export interface ExerciseInput {
  title: string;
  description: string | null;
  difficulty: number | null;
  code?: string | null;
  skill?: string | null;
  level?: string | null;
  tags: string[];
}

/** One entry of the wholesale-replace body of `PUT /library/lessons/:lid/materials`; body order is display order. */
export interface LessonMaterialInput {
  material_id: string;
  shared_with_students: boolean;
}

export interface LessonExerciseInput {
  exercise_id: string;
  /** Which exercise group of the lesson's own version this attaches to, if any. */
  group_id?: string | null;
}

/** One entry of `PUT /library/versions/:vid/log-fields`; options only matter for kind `select`. */
export interface LogFieldInput {
  label: string;
  kind: LogFieldKind;
  options: string[];
  required: boolean;
}

/** One component of a score set group; the key is what a class's grading refers to, unique within its own group. */
export interface ScoreComponentInput {
  key: string;
  label: string;
  max: number;
  weight: number;
}

/** One entry of `PUT /library/versions/:vid/score-set`; the group's key is its own stable identifier, unique within the set. */
export interface ScoreSetGroupInput {
  key: string;
  title: string;
  components: ScoreComponentInput[];
}

/** `library.ExerciseGroupRequest` — creates an exercise group of a draft version. */
export interface ExerciseGroupInput {
  name: string;
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
