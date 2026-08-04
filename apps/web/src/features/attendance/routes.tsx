import type { RouteObject } from "react-router";

/**
 * Mounted by the app router inside the protected dashboard layout, following
 * `apps/web/src/features/dashboard/routes.tsx`. `:id/attendance` nests under
 * `sessions` (rather than sitting as a flat sibling) so `SessionsPage` can
 * render it through `<Outlet/>` for the `lg+` two-pane layout while the same
 * route still resolves standalone at `/sessions/:id/attendance` under `lg`.
 */
export const attendanceRoutes: RouteObject[] = [
  {
    path: "sessions",
    lazy: async () => ({ Component: (await import("./pages/sessions-page")).SessionsPage }),
    children: [
      {
        path: ":id/attendance",
        lazy: async () => ({ Component: (await import("./pages/attendance-page")).AttendancePage }),
      },
    ],
  },
];
