import { useQueries } from "@tanstack/react-query";

import { getSessionScores } from "../api/teaching-api";
import { teachingKeys } from "./teaching-keys";

/**
 * Students with at least one component score, per held session. Only worth
 * asking when the class is configured with score components — the plain
 * general-score class reads its count from the month marks batch instead.
 * Keys match `useSessionScores`, so opening a row reuses the same cache entry.
 */
export function useSessionScoreCounts(
  sessionIds: readonly string[],
  enabled: boolean,
): Record<string, number> {
  const queries = useQueries({
    queries: sessionIds.map((sessionId) => ({
      queryKey: teachingKeys.sessionScores(sessionId),
      queryFn: () => getSessionScores(sessionId),
      enabled,
    })),
  });
  const counts: Record<string, number> = {};
  sessionIds.forEach((sessionId, index) => {
    const data = queries[index]?.data;
    if (!data) return;
    const students = new Set<string>();
    for (const entry of data.scores) {
      if (entry.score !== null) students.add(entry.student_id);
    }
    counts[sessionId] = students.size;
  });
  return counts;
}
