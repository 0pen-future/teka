import { apiClient } from "@/lib/api/client";
import { parseArray, parseData, parseList, type Paginated } from "@/lib/api/envelope";

import {
  coursePathSchema,
  learningPathSchema,
  type CoursePath,
  type LearningPath,
  type PathInput,
  type PathStatus,
  type StageInput,
} from "../schemas/paths-schemas";

export interface ListPathsParams {
  status?: PathStatus;
  q?: string;
  page?: number;
  per_page?: number;
  sort?: string;
}

export async function listPaths(params: ListPathsParams = {}): Promise<Paginated<LearningPath>> {
  const res = await apiClient.get("/paths", { params });
  return parseList(learningPathSchema, res.data);
}

export async function getPath(id: string): Promise<LearningPath> {
  const res = await apiClient.get(`/paths/${id}`);
  return parseData(learningPathSchema, res.data);
}

export async function createPath(input: PathInput): Promise<LearningPath> {
  const res = await apiClient.post("/paths", input);
  return parseData(learningPathSchema, res.data);
}

export async function updatePath(id: string, input: PathInput): Promise<LearningPath> {
  const res = await apiClient.put(`/paths/${id}`, input);
  return parseData(learningPathSchema, res.data);
}

export async function deletePath(id: string): Promise<void> {
  await apiClient.delete(`/paths/${id}`);
}

// Every stage call answers with the whole path, so the detail cache is
// replaced in one go and positions never drift from the server's numbering.

export async function createStage(pathId: string, input: StageInput): Promise<LearningPath> {
  const res = await apiClient.post(`/paths/${pathId}/stages`, input);
  return parseData(learningPathSchema, res.data);
}

export async function reorderStages(pathId: string, stageIds: string[]): Promise<LearningPath> {
  const res = await apiClient.put(`/paths/${pathId}/stages/order`, { stage_ids: stageIds });
  return parseData(learningPathSchema, res.data);
}

export async function updateStage(
  pathId: string,
  stageId: string,
  input: StageInput,
): Promise<LearningPath> {
  const res = await apiClient.put(`/paths/${pathId}/stages/${stageId}`, input);
  return parseData(learningPathSchema, res.data);
}

export async function deleteStage(pathId: string, stageId: string): Promise<LearningPath> {
  const res = await apiClient.delete(`/paths/${pathId}/stages/${stageId}`);
  return parseData(learningPathSchema, res.data);
}

export async function setStageCourses(
  pathId: string,
  stageId: string,
  courseIds: string[],
): Promise<LearningPath> {
  const res = await apiClient.put(`/paths/${pathId}/stages/${stageId}/courses`, {
    course_ids: courseIds,
  });
  return parseData(learningPathSchema, res.data);
}

/** The live paths whose stages recommend the course. */
export async function listCoursePaths(courseId: string): Promise<CoursePath[]> {
  const res = await apiClient.get(`/courses/${courseId}/paths`);
  return parseArray(coursePathSchema, res.data);
}
