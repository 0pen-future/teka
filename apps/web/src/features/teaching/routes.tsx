import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

/**
 * Mounted by the app router inside the protected dashboard layout. Pages
 * load through route.lazy so each lands in its own build chunk, following
 * `apps/web/src/features/roster/routes.tsx`.
 */
export const teachingRoutes: RouteObject[] = [
  {
    path: "classbook",
    handle: { title: "Sổ lớp" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/classbook-page")).ClassbookPage }),
  },
  {
    path: "records",
    handle: { title: "Hồ sơ học sinh" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/records-page")).RecordsPage }),
  },
  {
    path: "records/:studentId",
    handle: { title: "Hồ sơ của học sinh" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/student-record-page")).StudentRecordPage,
    }),
  },
  {
    path: "lesson-plans",
    handle: { title: "Duyệt giáo án" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/lesson-plans-page")).LessonPlansPage }),
  },
];
