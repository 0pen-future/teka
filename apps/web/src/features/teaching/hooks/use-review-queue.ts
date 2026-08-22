import { useQuery } from "@tanstack/react-query";

import { getReviewQueue } from "../api/teaching-api";
import type { QueueItemResponse } from "../schemas/teaching-schemas";
import { teachingKeys } from "./teaching-keys";
import { useCenterContext } from "./use-center-context";

/**
 * Pending-plan count for the sidebar nav dot — the review-queue endpoint's
 * only consumer (the lesson-plans page builds its own table from per-class
 * queries). Gated on the resolved owner role so a member account never even
 * issues the request (the server would 403 it); plan actions invalidate this
 * key, so approving/redoing drains the count. Same name and semantics as the
 * retired store hook: number-stable (`select` keeps re-renders to actual count
 * changes), 0 while the role or the queue is still resolving.
 */
export function usePendingPlanCount(): number {
  const { isOwner } = useCenterContext();
  const query = useQuery({
    queryKey: teachingKeys.reviewQueue(),
    queryFn: getReviewQueue,
    enabled: isOwner,
    select: (rows: QueueItemResponse[]) => rows.length,
  });
  return query.data ?? 0;
}
