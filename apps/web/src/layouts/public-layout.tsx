import { Outlet } from "react-router";

/**
 * Shell for the public, unauthenticated parent-statement route (`/s/:token`).
 * A centered column with no header, nav, or theme toggle — a parent lands
 * here straight from a Zalo link, with no session and no need for one.
 *
 * Deliberately imports nothing from `@/features/auth`, `@/features/roster`,
 * `@/features/billing`, `@/features/collections`, or the authenticated
 * `@/lib/api/client`: this route must keep rendering even if those modules
 * are broken or a session is entirely absent.
 */
export function PublicLayout() {
  return (
    <div id="main-content" className="mx-auto min-h-svh max-w-md px-4 py-6">
      <Outlet />
    </div>
  );
}
