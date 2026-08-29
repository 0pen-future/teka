import type { RouteObject } from "react-router";

/**
 * Mounted by the app router inside the protected dashboard layout. Lazy so
 * the page lands in its own build chunk, following
 * `apps/web/src/features/dashboard/routes.tsx`. No route guard: the nav entry
 * is gated on `canSendReports`, and the API answers a plain member with only
 * their own periods — the server is the authority.
 */
export const reportsRoutes: RouteObject[] = [
  {
    path: "reports",
    lazy: async () => ({
      Component: (await import("./pages/send-reports-page")).SendReportsPage,
    }),
  },
];
