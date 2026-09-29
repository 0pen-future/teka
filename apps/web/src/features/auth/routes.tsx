import type { RouteObject } from "react-router";

import type { RouteHandle } from "@/components/shared/document-title";

/**
 * Public-only routes; the app router mounts these under the auth layout.
 * Pages load through route.lazy so each lands in its own build chunk.
 */
export const authRoutes: RouteObject[] = [
  {
    path: "/login",
    handle: { title: "Đăng nhập" } satisfies RouteHandle,
    lazy: async () => ({ Component: (await import("./pages/login-page")).LoginPage }),
  },
  {
    path: "/forgot-password",
    handle: { title: "Quên mật khẩu" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/forgot-password-page")).ForgotPasswordPage,
    }),
  },
  {
    path: "/reset-password/:token",
    handle: { title: "Đặt lại mật khẩu" } satisfies RouteHandle,
    lazy: async () => ({
      Component: (await import("./pages/reset-password-page")).ResetPasswordPage,
    }),
  },
];
