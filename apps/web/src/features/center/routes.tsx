import type { RouteObject } from "react-router";

/**
 * Mounted by the app router inside the protected dashboard layout. Pages
 * load through route.lazy so each lands in its own build chunk, following
 * `apps/web/src/features/dashboard/routes.tsx`.
 */
export const centerRoutes: RouteObject[] = [
  {
    path: "center",
    lazy: async () => ({ Component: (await import("./pages/center-page")).CenterPage }),
  },
];
