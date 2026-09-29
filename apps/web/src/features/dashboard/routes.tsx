import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

/**
 * Mounted by the app router inside the protected dashboard layout.
 * Pages load through route.lazy so each lands in its own build chunk.
 */
export const dashboardRoutes: RouteObject[] = [
  {
    index: true,
    handle: { title: "Tổng quan" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/dashboard-page")).DashboardPage }),
  },
];
