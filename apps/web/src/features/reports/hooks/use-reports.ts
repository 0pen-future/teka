import { useQuery } from "@tanstack/react-query";

import { listReportPeriods } from "../api/reports-api";

export const reportsKeys = {
  all: ["reports"] as const,
  periods: () => [...reportsKeys.all, "periods"] as const,
};

/** The center-wide period list backing the reports oversight view. */
export function useReportPeriods() {
  return useQuery({
    queryKey: reportsKeys.periods(),
    queryFn: () => listReportPeriods(),
  });
}
