import type { RouteObject } from "react-router";

/** Mounted by the app router inside the protected dashboard layout; pages load lazily. */
export const coursesRoutes: RouteObject[] = [
  {
    path: "courses",
    lazy: async () => ({ Component: (await import("./pages/courses-page")).CoursesPage }),
  },
  {
    path: "courses/:id",
    lazy: async () => ({
      Component: (await import("./pages/course-detail-page")).CourseDetailPage,
    }),
  },
];
