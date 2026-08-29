import { apiClient } from "@/lib/api/client";
import { parseArray, parseData } from "@/lib/api/envelope";

import {
  classScoreComponentsSchema,
  scoreSetSchema,
  type ClassScoreComponents,
  type ScoreSet,
  type ScoreSetInput,
} from "../schemas/grading";

/** `GET /score-sets` — owner-only; the API 403s any other caller. */
export async function listScoreSets(): Promise<ScoreSet[]> {
  const res = await apiClient.get<unknown>("/score-sets");
  return parseArray(scoreSetSchema, res.data);
}

/** `POST /score-sets`. 409 on a duplicate name; 422 on invalid/blank/dup components. */
export async function createScoreSet(input: ScoreSetInput): Promise<ScoreSet> {
  const res = await apiClient.post<unknown>("/score-sets", input);
  return parseData(scoreSetSchema, res.data);
}

/** `PUT /score-sets/:id`. 404 missing set; 409 duplicate name; 422 invalid components. */
export async function updateScoreSet(id: string, input: ScoreSetInput): Promise<ScoreSet> {
  const res = await apiClient.put<unknown>(`/score-sets/${id}`, input);
  return parseData(scoreSetSchema, res.data);
}

/** `DELETE /score-sets/:id`. */
export async function deleteScoreSet(id: string): Promise<void> {
  await apiClient.delete(`/score-sets/${id}`);
}

/**
 * `POST /classes/:id/score-set` — copies the set's components onto the class
 * as a snapshot. Owner-only; 409 if the class already has recorded scores,
 * 404 if the class or set does not exist.
 */
export async function assignScoreSet(
  classId: string,
  setId: string,
): Promise<ClassScoreComponents> {
  const res = await apiClient.post<unknown>(`/classes/${classId}/score-set`, { set_id: setId });
  return parseData(classScoreComponentsSchema, res.data);
}

/** `DELETE /classes/:id/score-set` — owner-only; 409 if the class has recorded scores. */
export async function clearScoreSet(classId: string): Promise<void> {
  await apiClient.delete(`/classes/${classId}/score-set`);
}

/** `GET /classes/:id/score-components` — the class's currently assigned columns. */
export async function getClassComponents(classId: string): Promise<ClassScoreComponents> {
  const res = await apiClient.get<unknown>(`/classes/${classId}/score-components`);
  return parseData(classScoreComponentsSchema, res.data);
}
