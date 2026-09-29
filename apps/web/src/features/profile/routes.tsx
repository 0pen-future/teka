import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

/**
 * Mounted by the app router inside the protected dashboard layout. Pages
 * load through route.lazy so each lands in its own build chunk, following
 * `apps/web/src/features/dashboard/routes.tsx`.
 */
export const profileRoutes: RouteObject[] = [
  {
    path: "profile",
    handle: { title: "Hồ sơ cá nhân" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/profile-page")).ProfilePage }),
  },
];
