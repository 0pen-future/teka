import type { RouteObject } from "react-router";

/** Mounted by the app router inside the protected dashboard layout; pages load lazily. */
export const libraryRoutes: RouteObject[] = [
  {
    path: "library",
    lazy: async () => {
      const { LibraryPage } = await import("./pages/library-page");
      return { Component: () => <LibraryPage tab="templates" /> };
    },
  },
  {
    path: "library/materials",
    lazy: async () => {
      const { LibraryPage } = await import("./pages/library-page");
      return { Component: () => <LibraryPage tab="materials" /> };
    },
  },
  {
    path: "library/exercises",
    lazy: async () => {
      const { LibraryPage } = await import("./pages/library-page");
      return { Component: () => <LibraryPage tab="exercises" /> };
    },
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
];
