import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

/**
 * Mounted by the app router inside the protected dashboard layout. Pages
 * load through route.lazy so each lands in its own build chunk, following
 * `apps/web/src/features/dashboard/routes.tsx`.
 */
export const rosterRoutes: RouteObject[] = [
  {
    // The contact list lives in the students page's contacts tab now.
    path: "contacts",
    handle: { title: "Người liên hệ" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./components/contacts-redirect")).ContactsRedirect,
    }),
  },
  {
    path: "contacts/:id",
    handle: { title: "Chi tiết phụ huynh" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/contact-detail-page")).ContactDetailPage,
    }),
  },
  {
    path: "students",
    handle: { title: "Học sinh" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/students-page")).StudentsPage }),
  },
  {
    // Also matched by "students/:id" below; react-router ranks the static
    // segment higher, so /students/import never resolves "import" as an id.
    path: "students/import",
    handle: { title: "Nhập từ Excel" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/roster-import-page")).RosterImportPage,
    }),
  },
  {
    path: "students/:id",
    handle: { title: "Chi tiết học sinh" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/student-detail-page")).StudentDetailPage,
    }),
  },
  {
    path: "classes",
    handle: { title: "Danh mục lớp" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/class-list-page")).ClassListPage }),
  },
  {
    // Static segment, so react-router ranks it above "classes/:id". The
    // recruiting list lives in the catalog's "Cần tuyển sinh" chip now; the
    // title matches the catalog so it does not flicker through the redirect.
    path: "classes/recruiting",
    handle: { title: "Danh mục lớp" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./components/recruiting-redirect")).RecruitingRedirect,
    }),
  },
  {
    path: "classes/:id",
    handle: { title: "Chi tiết lớp" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/class-detail-page")).ClassDetailPage,
    }),
  },
  {
    path: "class-invitations",
    handle: { title: "Lời mời nhận lớp" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/class-invitations-page")).ClassInvitationsPage,
    }),
  },
  {
    path: "classes/:id/settings",
    handle: { title: "Chi tiết lớp" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./components/class-settings-redirect")).ClassSettingsRedirect,
    }),
  },
];
