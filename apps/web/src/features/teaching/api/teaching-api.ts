import { apiClient } from "@/lib/api/client";
import { parseArray, parseData } from "@/lib/api/envelope";

import {
  curriculumResponseSchema,
  markResponseSchema,
  monthMarksResponseSchema,
  noteResponseSchema,
  planResponseSchema,
  queueItemResponseSchema,
  type CurriculumResponse,
  type MarkEntryInput,
  type MarkResponse,
  type MonthMarksResponse,
  type NoteResponse,
  type PlanActionName,
  type PlanResponse,
  type PutCurriculumInput,
  type QueueItemResponse,
  type SavePlanInput,
} from "../schemas/teaching-schemas";

/**
 * `GET /classes/:id/curriculum` (`apps/api/internal/features/teaching/
 * handler.go`); a class that never saved one answers the empty default.
 */
export async function getCurriculum(classId: string): Promise<CurriculumResponse> {
  const res = await apiClient.get<unknown>(`/classes/${classId}/curriculum`);
  return parseData(curriculumResponseSchema, res.data);
}

/** `PUT /classes/:id/curriculum` — whole-replaces the lesson list (class teacher only). */
export async function putCurriculum(
  classId: string,
  input: PutCurriculumInput,
): Promise<CurriculumResponse> {
  const res = await apiClient.put<unknown>(`/classes/${classId}/curriculum`, input);
  return parseData(curriculumResponseSchema, res.data);
}

/** `GET /classes/:id/lesson-plans` — every saved giáo án of the class, no meta block. */
export async function listPlans(classId: string): Promise<PlanResponse[]> {
  const res = await apiClient.get<unknown>(`/classes/${classId}/lesson-plans`);
  return parseArray(planResponseSchema, res.data);
}

/** `PUT /classes/:id/lesson-plans/:index` — teacher save; status stays with the state machine. */
export async function savePlan(
  classId: string,
  lessonIndex: number,
  input: SavePlanInput,
): Promise<PlanResponse> {
  const res = await apiClient.put<unknown>(
    `/classes/${classId}/lesson-plans/${lessonIndex}`,
    input,
  );
  return parseData(planResponseSchema, res.data);
}

/**
 * `POST /classes/:id/lesson-plans/:index/{submit|approve|request-redo|reopen}`
 * — the review loop. An illegal transition answers 409; request-redo requires
 * a non-blank comment (422).
 */
export async function planAction(
  classId: string,
  lessonIndex: number,
  action: PlanActionName,
  comment?: string,
): Promise<PlanResponse> {
  const res = await apiClient.post<unknown>(
    `/classes/${classId}/lesson-plans/${lessonIndex}/${action}`,
    comment === undefined ? {} : { comment },
  );
  return parseData(planResponseSchema, res.data);
}

/**
 * `GET /classes/:id/marks?month=YYYY-MM` — every session note and mark row of
 * the class's sessions in that month, in one read.
 */
export async function getMonthMarks(classId: string, month: string): Promise<MonthMarksResponse> {
  const res = await apiClient.get<unknown>(`/classes/${classId}/marks`, { params: { month } });
  return parseData(monthMarksResponseSchema, res.data);
}

/** `PUT /sessions/:id/note` — upsert; an empty/whitespace body deletes the note. */
export async function putNote(sessionId: string, body: string): Promise<NoteResponse> {
  const res = await apiClient.put<unknown>(`/sessions/${sessionId}/note`, { body });
  return parseData(noteResponseSchema, res.data);
}

/**
 * `PUT /sessions/:id/marks` — tri-state batch (see `MarkEntryInput`); answers
 * the session's full post-write mark set.
 */
export async function putMarks(
  sessionId: string,
  entries: MarkEntryInput[],
): Promise<MarkResponse[]> {
  const res = await apiClient.put<unknown>(`/sessions/${sessionId}/marks`, entries);
  return parseArray(markResponseSchema, res.data);
}

/** `GET /teaching/review-queue` — the center's pending giáo án (owner only, 403 otherwise). */
export async function getReviewQueue(): Promise<QueueItemResponse[]> {
  const res = await apiClient.get<unknown>(`/teaching/review-queue`);
  return parseArray(queueItemResponseSchema, res.data);
}
