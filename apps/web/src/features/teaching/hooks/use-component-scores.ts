import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";

import { hvToast } from "@/components/hv";

import { getClassScoreComponents, getSessionScores, putSessionScores } from "../api/teaching-api";
import type {
  PutSessionScoreEntryInput,
  SessionScoreEntry,
  SessionScoresResponse,
} from "../schemas/teaching-schemas";
import { teachingKeys } from "./teaching-keys";

/**
 * Whether a class is configured with score components, and which. An empty
 * `components` array is the "class uses the plain general-score entry"
 * signal — callers branch on `data?.components.length`, not on
 * `isPending`/`isError`, so a still-loading or unconfigured class quietly
 * falls back to the old block instead of showing an error state.
 */
export function useClassScoreComponents(classId: string | undefined) {
  return useQuery({
    queryKey: teachingKeys.scoreComponents(classId ?? ""),
    queryFn: () => getClassScoreComponents(classId!),
    enabled: Boolean(classId),
  });
}

/** One session's component set plus whatever student×component cells are filled in. */
export function useSessionScores(sessionId: string | undefined) {
  return useQuery({
    queryKey: teachingKeys.sessionScores(sessionId ?? ""),
    queryFn: () => getSessionScores(sessionId!),
    enabled: Boolean(sessionId),
  });
}

/**
 * PUT returns the session's FULL current score set — a cleared cell is simply
 * absent from it, never echoed as `null`. So the write is a wholesale replace,
 * not a per-cell merge: keeping the old rows around would leave a just-cleared
 * cell stuck showing its previous value. Mirrors `useSaveMarks`.
 */
function writeSessionScoresToCache(
  queryClient: QueryClient,
  sessionId: string,
  updated: SessionScoreEntry[],
): void {
  queryClient.setQueryData<SessionScoresResponse>(
    teachingKeys.sessionScores(sessionId),
    (data) => data && { ...data, scores: updated },
  );
}

/**
 * Batch save changed student×component cells for one session. Writes the
 * cache from the server's full echoed set on success — mirrors `useSaveMarks` —
 * so the grid reflects the saved state (cleared cells included) without a
 * refetch flash; a failed save invalidates instead of guessing and surfaces
 * the repo's standard danger toast.
 */
export function useSaveSessionScores(sessionId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (entries: PutSessionScoreEntryInput[]) => putSessionScores(sessionId, entries),
    onSuccess: (rows) => writeSessionScoresToCache(queryClient, sessionId, rows),
    onError: () => {
      void queryClient.invalidateQueries({ queryKey: teachingKeys.sessionScores(sessionId) });
      hvToast("Không lưu được điểm thành phần — vui lòng thử lại", { variant: "danger" });
    },
  });
}
