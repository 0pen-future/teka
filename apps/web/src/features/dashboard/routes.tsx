import type { RouteObject } from "react-router";

/**
 * Mounted by the app router inside the protected dashboard layout.
 * Pages load through route.lazy so each lands in its own build chunk.
 */
export const dashboardRoutes: RouteObject[] = [
  {
    index: true,
    lazy: async () => ({ Component: (await import("./pages/dashboard-page")).DashboardPage }),
  },
];
