import type { RouteObject } from "react-router";

/**
 * Public-only route; the app router mounts this under the auth layout
 * alongside `authRoutes` (login has nothing to do with accepting an invite,
 * but both are "no session required" pages). Lazy-loaded so the page lands
 * in its own build chunk, following `apps/web/src/features/auth/routes.tsx`.
 */
export const invitationRoutes: RouteObject[] = [
  {
    path: "/invite/:token",
    lazy: async () => ({
      Component: (await import("./pages/accept-invite-page")).AcceptInvitePage,
    }),
  },
];
