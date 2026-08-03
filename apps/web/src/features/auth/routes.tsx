import type { RouteObject } from "react-router";

/**
 * Public-only routes; the app router mounts these under the auth layout.
 * Pages load through route.lazy so each lands in its own build chunk.
 */
export const authRoutes: RouteObject[] = [
  {
    path: "/login",
    lazy: async () => ({ Component: (await import("./pages/login-page")).LoginPage }),
  },
  {
    path: "/register",
    lazy: async () => ({ Component: (await import("./pages/register-page")).RegisterPage }),
  },
];
