import type { RouteObject } from "react-router";

/**
 * Mounted by the app router inside the protected dashboard layout.
 * Pages load through route.lazy so each lands in its own build chunk.
 */
export const usersRoutes: RouteObject[] = [
  {
    path: "users",
    lazy: async () => ({ Component: (await import("./pages/users-page")).UsersPage }),
  },
  {
    path: "users/:id",
    lazy: async () => ({ Component: (await import("./pages/user-detail-page")).UserDetailPage }),
  },
];
