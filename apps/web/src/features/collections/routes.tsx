import type { RouteObject } from "react-router";

/**
 * Mounted by the app router inside the protected dashboard layout. Pages
 * load through route.lazy so each lands in its own build chunk, following
 * `apps/web/src/features/dashboard/routes.tsx`. Covers both screens this
 * feature owns: "Thu tiền" (`collections/:periodId`) and "Gửi thông báo"
 * (`notifications/:periodId`) — see the phase spec's flow diagram, which
 * places notifications as a sibling top-level path rather than nested under
 * `collections`.
 */
export const collectionsRoutes: RouteObject[] = [
  {
    path: "collections/:periodId",
    lazy: async () => ({ Component: (await import("./pages/collections-page")).CollectionsPage }),
  },
  {
    path: "notifications/:periodId",
    lazy: async () => ({
      Component: (await import("./pages/notifications-page")).NotificationsPage,
    }),
  },
];
