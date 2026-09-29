import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

/** Mounted by the app router inside the protected dashboard layout; pages load lazily. */
export const coursesRoutes: RouteObject[] = [
  {
    path: "courses",
    handle: { title: "Khóa học" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/courses-page")).CoursesPage }),
  },
  {
    path: "courses/:id",
    handle: { title: "Chi tiết khóa học" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/course-detail-page")).CourseDetailPage,
    }),
  },
  {
    path: "paths",
    handle: { title: "Lộ trình học" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/learning-paths-page")).LearningPathsPage,
    }),
  },
  {
    path: "paths/:id",
    handle: { title: "Chi tiết lộ trình" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/path-detail-page")).PathDetailPage }),
  },
];
