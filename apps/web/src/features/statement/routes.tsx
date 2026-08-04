import type { RouteObject } from "react-router";

/**
 * Path prefix for the public parent-statement route. Exported as the single
 * source of truth so `SessionRestore`'s path-based skip check
 * (`apps/web/src/features/auth/components/session-restore.tsx`) cannot drift
 * from where this feature actually mounts.
 */
export const STATEMENT_PATH_PREFIX = "/s/";

/**
 * Mounted by the app router under `PublicLayout`, outside `ProtectedRoute`
 * and outside the authenticated dashboard tree. Lazy-loaded so the statement
 * view lands in its own chunk, following
 * `apps/web/src/features/billing/routes.tsx`.
 */
export const statementRoutes: RouteObject[] = [
  {
    path: `${STATEMENT_PATH_PREFIX}:token`,
    lazy: async () => ({ Component: (await import("./pages/statement-page")).StatementPage }),
  },
];
