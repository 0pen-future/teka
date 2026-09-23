import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createPath,
  createStage,
  deletePath,
  deleteStage,
  getPath,
  listCoursePaths,
  listPaths,
  reorderStages,
  setStageCourses,
  updatePath,
  updateStage,
  type ListPathsParams,
} from "../api/paths-api";
import type { LearningPath, PathInput, StageInput } from "../schemas/paths-schemas";
import { pathsKeys } from "./paths-keys";

export function usePathsList(params: ListPathsParams = {}, enabled = true) {
  return useQuery({
    queryKey: pathsKeys.list(params),
    queryFn: () => listPaths(params),
    placeholderData: keepPreviousData,
    enabled,
  });
}

export function usePath(id: string) {
  return useQuery({
    queryKey: pathsKeys.detail(id),
    queryFn: () => getPath(id),
    enabled: id !== "",
  });
}

/** Lists the paths recommending a course; off until the reader holds `paths.read`. */
export function useCoursePaths(courseId: string, enabled = true) {
  return useQuery({
    queryKey: pathsKeys.byCourse(courseId),
    queryFn: () => listCoursePaths(courseId),
    enabled: enabled && courseId !== "",
  });
}

export function useCreatePath() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: createPath,
    onSuccess: (path: LearningPath) => {
      queryClient.setQueryData(pathsKeys.detail(path.id), path);
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: pathsKeys.lists() });
    },
  });
}

export function useUpdatePath(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: PathInput) => updatePath(id, input),
    onSuccess: (path: LearningPath) => {
      queryClient.setQueryData(pathsKeys.detail(id), path);
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: pathsKeys.lists() });
    },
  });
}

export function useDeletePath() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deletePath,
    onSuccess: (_, id) => {
      queryClient.removeQueries({ queryKey: pathsKeys.detail(id) });
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: pathsKeys.lists() });
    },
  });
}

/**
 * Stage mutations all hand back the whole path: the detail cache takes it
 * as is, and the list (its counters) plus the per-course listings refetch.
 */
function useStageMutation<TVariables>(
  pathId: string,
  mutationFn: (variables: TVariables) => Promise<LearningPath>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: (path: LearningPath) => {
      queryClient.setQueryData(pathsKeys.detail(pathId), path);
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: pathsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: [...pathsKeys.all, "by-course"] });
    },
  });
}

export function useCreateStage(pathId: string) {
  return useStageMutation(pathId, (input: StageInput) => createStage(pathId, input));
}

export function useUpdateStage(pathId: string) {
  return useStageMutation(pathId, (vars: { stageId: string; input: StageInput }) =>
    updateStage(pathId, vars.stageId, vars.input),
  );
}

export function useDeleteStage(pathId: string) {
  return useStageMutation(pathId, (stageId: string) => deleteStage(pathId, stageId));
}

export function useReorderStages(pathId: string) {
  return useStageMutation(pathId, (stageIds: string[]) => reorderStages(pathId, stageIds));
}

export function useSetStageCourses(pathId: string) {
  return useStageMutation(pathId, (vars: { stageId: string; courseIds: string[] }) =>
    setStageCourses(pathId, vars.stageId, vars.courseIds),
  );
}
