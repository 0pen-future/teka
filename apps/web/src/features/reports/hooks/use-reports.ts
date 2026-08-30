import { useQuery } from "@tanstack/react-query";

import { listReportPeriods } from "../api/reports-api";

export const reportsKeys = {
  all: ["reports"] as const,
  periods: (classId?: string) => [...reportsKeys.all, "periods", classId ?? "center"] as const,
};

/**
 * Without `classId`: the center-wide period list (reports oversight view).
 * With it: only periods carrying that class's charges — a hoc_vu's period
 * discovery for the class-scoped send. The server 404s a caller without a
 * stint on the class, so callers gate on `canSendClassReports` first.
 */
export function useReportPeriods(classId?: string) {
  return useQuery({
    queryKey: reportsKeys.periods(classId),
    queryFn: () => listReportPeriods(classId),
  });
}
