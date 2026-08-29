import { z } from "zod";

/**
 * Wire schemas for the teaching API (`apps/api/internal/features/teaching/
 * dto.go`). The store-shaped types the components consume (`Curriculum`,
 * `LessonPlan`, `TeachingState`) stay in `../lib/teaching-store`; the hooks
 * adapt between the two so no component ever sees a wire shape.
 */

/**
 * `teaching.CurriculumResponse`. A class that never saved a curriculum reads
 * as the empty default (`lessons: [], current_index: 0`), not a 404.
 */
export const curriculumResponseSchema = z.object({
  lessons: z.array(z.string()),
  current_index: z.number().int().nonnegative(),
});

export type CurriculumResponse = z.infer<typeof curriculumResponseSchema>;

/** `teaching.PutCurriculumRequest` — whole-replace; the server clamps `current_index`. */
export interface PutCurriculumInput {
  lessons: string[];
  current_index: number;
}

/**
 * Server-side plan statuses. `"none"` never travels the wire — it is the
 * absence of a row, mapped back in by the `useClassTeaching` adapter.
 */
export const apiPlanStatusSchema = z.enum(["draft", "pending", "approved", "redo"]);

/** `teaching.PlanResponse` — one giáo án. */
export const planResponseSchema = z.object({
  class_id: z.string(),
  lesson_index: z.number().int().nonnegative(),
  goal: z.string(),
  activities: z.array(z.string()),
  homework: z.string(),
  file_name: z.string().nullable(),
  status: apiPlanStatusSchema,
  redo_note: z.string().nullable(),
  owner_comment: z.string().nullable(),
  submitted_by: z.string().nullable(),
  submitted_by_name: z.string().nullable(),
  submitted_at: z.string().nullable(),
});

export type PlanResponse = z.infer<typeof planResponseSchema>;

/** `teaching.SavePlanRequest` — full content replace; `file_name: null` clears the attachment. */
export interface SavePlanInput {
  goal: string;
  activities: string[];
  homework: string;
  file_name: string | null;
}

/** The four review-loop POST actions; the server's state machine 409s illegal moves. */
export type PlanActionName = "submit" | "approve" | "request-redo" | "reopen";

/** `teaching.NoteResponse` — `body: ""` means no note exists (or it was just deleted). */
export const noteResponseSchema = z.object({
  session_id: z.string(),
  body: z.string(),
});

export type NoteResponse = z.infer<typeof noteResponseSchema>;

/** `teaching.MarkResponse` — one student's score and/or personal note for one session. */
export const markResponseSchema = z.object({
  session_id: z.string(),
  student_id: z.string(),
  score: z.number().nullable(),
  personal_note: z.string().nullable(),
});

export type MarkResponse = z.infer<typeof markResponseSchema>;

/**
 * `teaching.MarkEntryRequest` — tri-state per field: key omitted = leave the
 * stored value untouched, `null` = clear it, value = set it. A row whose
 * resulting fields are both NULL is deleted server-side.
 */
export interface MarkEntryInput {
  student_id: string;
  score?: number | null;
  personal_note?: string | null;
}

/** `teaching.MonthMarksResponse` — the classbook/records batch read. */
export const monthMarksResponseSchema = z.object({
  session_notes: z.array(noteResponseSchema),
  marks: z.array(markResponseSchema),
});

export type MonthMarksResponse = z.infer<typeof monthMarksResponseSchema>;

/**
 * `teaching.QueueItemResponse` — one pending giáo án in the owner's review
 * queue. `lesson_title`/`teacher_name` are null-safe (curriculum shrank, or
 * the submitter is unknown); the web falls back to its own placeholders.
 */
export const queueItemResponseSchema = z.object({
  plan_id: z.string(),
  class_id: z.string(),
  class_name: z.string(),
  lesson_index: z.number().int().nonnegative(),
  lesson_title: z.string().nullable(),
  teacher_name: z.string().nullable(),
  submitted_at: z.string().nullable(),
});

export type QueueItemResponse = z.infer<typeof queueItemResponseSchema>;

/** One configured score column for a class (`teaching.ClassScoreComponentResponse`). */
export const classScoreComponentSchema = z.object({
  id: z.string(),
  name: z.string(),
  position: z.number().int(),
});

export type ClassScoreComponent = z.infer<typeof classScoreComponentSchema>;

/**
 * `GET /classes/:id/score-components`. An empty `components` array is the
 * signal the class uses the plain general-score entry instead of the
 * per-component grid — never treated as an error state.
 */
export const classScoreComponentsResponseSchema = z.object({
  class_id: z.string(),
  components: z.array(classScoreComponentSchema),
});

export type ClassScoreComponentsResponse = z.infer<typeof classScoreComponentsResponseSchema>;

/**
 * One student×component cell, from `GET /sessions/:id/scores` or the `PUT`
 * echo. `score` is nullable defensively (mirrors `MarkResponse`) even though
 * the GET read only ever lists filled cells.
 */
export const sessionScoreEntrySchema = z.object({
  student_id: z.string(),
  component_id: z.string(),
  score: z.number().nullable(),
});

export type SessionScoreEntry = z.infer<typeof sessionScoreEntrySchema>;

/** `GET /sessions/:id/scores` — the session's component set plus filled-in cells. */
export const sessionScoresResponseSchema = z.object({
  components: z.array(classScoreComponentSchema),
  scores: z.array(sessionScoreEntrySchema),
});

export type SessionScoresResponse = z.infer<typeof sessionScoresResponseSchema>;

/** `PUT /sessions/:id/scores` request row — `score: null` clears that cell. */
export interface PutSessionScoreEntryInput {
  student_id: string;
  component_id: string;
  score: number | null;
}
