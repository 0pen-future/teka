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
    handle: { title: "Học sinh" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./components/records-redirect")).RecordsRedirect,
    }),
  },
  {
    path: "records/:studentId",
    handle: { title: "Chi tiết học sinh" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./components/records-redirect")).StudentRecordRedirect,
    }),
  },
  {
    path: "lesson-plans",
    handle: { title: "Duyệt giáo án" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/lesson-plans-page")).LessonPlansPage }),
  },
];
