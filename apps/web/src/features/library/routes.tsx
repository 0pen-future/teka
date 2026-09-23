import type { RouteObject } from "react-router";

/** Mounted by the app router inside the protected dashboard layout; pages load lazily. */
export const libraryRoutes: RouteObject[] = [
  {
    path: "library",
    lazy: async () => ({ Component: (await import("./pages/library-page")).LibraryPage }),
  },
  {
    path: "library/templates/:id",
    lazy: async () => ({
      Component: (await import("./pages/template-detail-page")).TemplateDetailPage,
    }),
  },
  {
    path: "library/templates/:id/lessons/:lessonId",
    lazy: async () => ({
      Component: (await import("./pages/template-lesson-page")).TemplateLessonPage,
    }),
  },
];
