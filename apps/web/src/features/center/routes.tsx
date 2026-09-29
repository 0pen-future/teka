import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

/**
 * Mounted by the app router inside the protected dashboard layout. Pages
 * load through route.lazy so each lands in its own build chunk, following
 * `apps/web/src/features/dashboard/routes.tsx`.
 */
export const centerRoutes: RouteObject[] = [
  {
    path: "center",
    handle: { title: "Cài đặt trung tâm" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/center-page")).CenterPage }),
  },
  {
    path: "center/permissions",
    handle: { title: "Phân quyền vai trò" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/center-permissions-page")).CenterPermissionsPage,
    }),
  },
  {
    path: "center/class-config",
    handle: { title: "Cài đặt trung tâm" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./components/class-config-redirect")).ClassConfigRedirect,
    }),
  },
];
