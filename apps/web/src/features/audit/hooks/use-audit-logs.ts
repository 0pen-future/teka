import { useInfiniteQuery } from "@tanstack/react-query";

import { listAuditLogs } from "../api/audit-api";
import type { AuditLogFilters } from "../schemas/audit-schemas";

export const auditKeys = {
  all: ["audit"] as const,
  lists: () => [...auditKeys.all, "list"] as const,
  list: (filters: AuditLogFilters) => [...auditKeys.lists(), filters] as const,
};

/**
 * Cursor-paged audit trail. The filters live in the query key, so any filter
 * change discards fetched pages and restarts from the first page — a cursor
 * is only valid alongside the filters that produced it. `enabled` gates the
 * request off entirely for non-owners, whose call would only 403.
 */
export function useAuditLogs(filters: AuditLogFilters, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: auditKeys.list(filters),
    queryFn: ({ pageParam }) => listAuditLogs(filters, pageParam || undefined),
    initialPageParam: "",
    getNextPageParam: (lastPage) =>
      lastPage.next_cursor === "" ? undefined : lastPage.next_cursor,
    enabled,
  });
}
