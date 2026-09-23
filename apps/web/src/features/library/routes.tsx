import type { RouteObject } from "react-router";

/** Mounted by the app router inside the protected dashboard layout; pages load lazily. */
export const libraryRoutes: RouteObject[] = [
  {
    path: "library",
    lazy: async () => ({ Component: (await import("./pages/library-page")).LibraryPage }),
  },
  {
    path: "library/templates/new",
    lazy: async () => ({
      Component: (await import("./pages/template-create-wizard-page")).TemplateCreateWizardPage,
    }),
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
  {
    path: "prep",
    lazy: async () => ({ Component: (await import("./pages/prep-page")).PrepPage }),
  },
  {
    path: "prep/:vid/board",
    lazy: async () => ({ Component: (await import("./pages/prep-board-page")).PrepBoardPage }),
  },
  {
    path: "prep/:vid/assign",
    lazy: async () => ({
      Component: (await import("./pages/prep-assign-page")).PrepAssignPage,
    }),
  },
];
