import { useQuery } from "@tanstack/react-query";

import { getPendingSessions } from "../api/dashboard-api";

export const dashboardKeys = {
  all: ["dashboard"] as const,
  pendingSessions: () => [...dashboardKeys.all, "pending-sessions"] as const,
};

export function usePendingSessions() {
  return useQuery({
    queryKey: dashboardKeys.pendingSessions(),
    queryFn: getPendingSessions,
  });
}
