import { apiClient } from "@/lib/api/client";
import { parseArray, parseData } from "@/lib/api/envelope";

import {
  classScoreComponentsResponseSchema,
  curriculumResponseSchema,
  markResponseSchema,
  monthMarksResponseSchema,
  noteResponseSchema,
  planResponseSchema,
  queueItemResponseSchema,
  sessionScoreEntrySchema,
  sessionScoresResponseSchema,
  type ClassScoreComponentsResponse,
  type CurriculumResponse,
  type MarkEntryInput,
  type MarkResponse,
  type MonthMarksResponse,
  type NoteResponse,
  type PlanActionName,
  type PlanResponse,
  type PutCurriculumInput,
  type PutSessionScoreEntryInput,
  type QueueItemResponse,
  type SavePlanInput,
  type SessionScoreEntry,
  type SessionScoresResponse,
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

/**
 * `GET /classes/:id/score-components` — an empty `components` list means the
 * class uses the plain general-score entry, not the per-component grid.
 */
export async function getClassScoreComponents(
  classId: string,
): Promise<ClassScoreComponentsResponse> {
  const res = await apiClient.get<unknown>(`/classes/${classId}/score-components`);
  return parseData(classScoreComponentsResponseSchema, res.data);
}

/** `GET /sessions/:id/scores` — the session's component×student score grid. */
export async function getSessionScores(sessionId: string): Promise<SessionScoresResponse> {
  const res = await apiClient.get<unknown>(`/sessions/${sessionId}/scores`);
  return parseData(sessionScoresResponseSchema, res.data);
}

/**
 * `PUT /sessions/:id/scores` — batch upsert/clear of student×component
 * cells; the request body is a bare array, not wrapped in an object. Writable
 * by the session's teacher or the center owner.
 */
export async function putSessionScores(
  sessionId: string,
  entries: PutSessionScoreEntryInput[],
): Promise<SessionScoreEntry[]> {
  const res = await apiClient.put<unknown>(`/sessions/${sessionId}/scores`, entries);
  return parseArray(sessionScoreEntrySchema, res.data);
}
