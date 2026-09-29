import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

/** Mounted by the app router inside the protected dashboard layout; pages load lazily. */
export const libraryRoutes: RouteObject[] = [
  {
    path: "library",
    handle: { title: "Chương trình mẫu" } satisfies RouteHandle,
    lazy: async () => {
      const { LibraryPage } = await import("./pages/library-page");
      return { Component: () => <LibraryPage tab="templates" /> };
    },
  },
  {
    path: "library/materials",
    handle: { title: "Ngân hàng nội dung" } satisfies RouteHandle,
    lazy: async () => {
      const { LibraryPage } = await import("./pages/library-page");
      return { Component: () => <LibraryPage tab="materials" /> };
    },
  },
  {
    path: "library/exercises",
    handle: { title: "Ngân hàng bài tập" } satisfies RouteHandle,
    lazy: async () => {
      const { LibraryPage } = await import("./pages/library-page");
      return { Component: () => <LibraryPage tab="exercises" /> };
    },
  },
  {
    path: "library/templates/new",
    handle: { title: "Tạo chương trình mẫu" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/template-create-wizard-page")).TemplateCreateWizardPage,
    }),
  },
  {
    path: "library/templates/:id",
    handle: { title: "Chi tiết chương trình mẫu" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/template-detail-page")).TemplateDetailPage,
    }),
  },
  {
    path: "library/templates/:id/lessons/:lessonId",
    handle: { title: "Bài học mẫu" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/template-lesson-page")).TemplateLessonPage,
    }),
  },
];
