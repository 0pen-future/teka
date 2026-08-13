import type { RouteObject } from "react-router";

/**
 * Mounted by the app router inside the protected dashboard layout. Pages
 * load through route.lazy so each lands in its own build chunk, following
 * `apps/web/src/features/roster/routes.tsx`.
 */
export const teachingRoutes: RouteObject[] = [
  {
    path: "classbook",
    lazy: async () => ({ Component: (await import("./pages/classbook-page")).ClassbookPage }),
  },
  {
    path: "records",
    lazy: async () => ({ Component: (await import("./pages/records-page")).RecordsPage }),
  },
  {
    path: "records/:studentId",
    lazy: async () => ({
      Component: (await import("./pages/student-record-page")).StudentRecordPage,
    }),
  },
  {
    path: "lesson-plans",
    lazy: async () => ({ Component: (await import("./pages/lesson-plans-page")).LessonPlansPage }),
  },
];
