import type { RouteObject } from "react-router";

/**
 * Mounted by the app router inside the protected dashboard layout. Pages
 * load through route.lazy so each lands in its own build chunk, following
 * `apps/web/src/features/dashboard/routes.tsx`.
 */
export const rosterRoutes: RouteObject[] = [
  {
    path: "contacts",
    lazy: async () => ({ Component: (await import("./pages/contacts-page")).ContactsPage }),
  },
  {
    path: "contacts/:id",
    lazy: async () => ({
      Component: (await import("./pages/contact-detail-page")).ContactDetailPage,
    }),
  },
  {
    path: "students",
    lazy: async () => ({ Component: (await import("./pages/students-page")).StudentsPage }),
  },
  {
    // Also matched by "students/:id" below; react-router ranks the static
    // segment higher, so /students/import never resolves "import" as an id.
    path: "students/import",
    lazy: async () => ({
      Component: (await import("./pages/roster-import-page")).RosterImportPage,
    }),
  },
  {
    path: "students/:id",
    lazy: async () => ({
      Component: (await import("./pages/student-detail-page")).StudentDetailPage,
    }),
  },
  {
    path: "classes/:id/settings",
    lazy: async () => ({
      Component: (await import("./pages/class-settings-page")).ClassSettingsPage,
    }),
  },
];
