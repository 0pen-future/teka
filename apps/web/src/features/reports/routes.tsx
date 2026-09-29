import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

/**
 * Mounted by the app router inside the protected dashboard layout. Lazy so
 * the page lands in its own build chunk, following
 * `apps/web/src/features/dashboard/routes.tsx`. No route guard: the "Gửi
 * thông báo" nav entry is gated on `reports.send` and is the one entry point
 * for every role, the owner included. The server is the authority: the owner
 * and `reports.send` holders see every center period through
 * `billing.view_all` (implied by `reports.send`), while a plain member sees
 * only their own periods.
 */
export const reportsRoutes: RouteObject[] = [
  {
    path: "reports",
    handle: { title: "Gửi thông báo" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/send-reports-page")).SendReportsPage,
    }),
  },
];
