import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  assignScoreSet,
  clearScoreSet,
  createScoreSet,
  deleteScoreSet,
  getClassComponents,
  listScoreSets,
  updateScoreSet,
} from "../api/grading";
import type { ScoreSetInput } from "../schemas/grading";

export const gradingKeys = {
  scoreSets: ["center", "score-sets"] as const,
  classComponents: (classId: string) => ["center", "score-components", classId] as const,
};

/** Owner-only read model; callers gate on the owner shape before mounting. */
export function useScoreSets() {
  return useQuery({ queryKey: gradingKeys.scoreSets, queryFn: listScoreSets });
}

function useScoreSetMutation<TVariables>(mutationFn: (variables: TVariables) => Promise<unknown>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: gradingKeys.scoreSets });
    },
  });
}

export function useCreateScoreSet() {
  return useScoreSetMutation((input: ScoreSetInput) => createScoreSet(input));
}

export function useUpdateScoreSet() {
  return useScoreSetMutation(({ id, input }: { id: string; input: ScoreSetInput }) =>
    updateScoreSet(id, input),
  );
}

export function useDeleteScoreSet() {
  return useScoreSetMutation((id: string) => deleteScoreSet(id));
}

/** The class's currently assigned columns; not tied to any `ScoreSet` id (see schema note). */
export function useClassScoreComponents(classId: string | undefined) {
  return useQuery({
    queryKey: gradingKeys.classComponents(classId ?? ""),
    queryFn: () => getClassComponents(classId!),
    enabled: Boolean(classId),
  });
}

export function useAssignScoreSet() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ classId, setId }: { classId: string; setId: string }) =>
      assignScoreSet(classId, setId),
    onSuccess: (_result, { classId }) => {
      void queryClient.invalidateQueries({ queryKey: gradingKeys.classComponents(classId) });
    },
  });
}

export function useClearScoreSet() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (classId: string) => clearScoreSet(classId),
    onSuccess: (_result, classId) => {
      void queryClient.invalidateQueries({ queryKey: gradingKeys.classComponents(classId) });
    },
  });
}
