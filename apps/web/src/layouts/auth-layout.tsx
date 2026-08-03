import { Navigate, Outlet, useLocation } from "react-router";

import { useIsAuthenticated } from "@/features/auth/stores/auth-store";

/**
 * Public-only pages (login, register): authenticated users are sent on.
 * Honoring state.from here (not only in the login handler) keeps the
 * store-update/navigate race harmless — both paths land on the same target.
 */
export function AuthLayout() {
  const isAuthenticated = useIsAuthenticated();
  const location = useLocation();

  if (isAuthenticated) {
    const from = (location.state as { from?: string } | null)?.from;
    return <Navigate to={from ?? "/"} replace />;
  }
  return (
    <main id="main-content" className="flex min-h-svh items-center justify-center bg-muted/40 p-4">
      <div className="w-full max-w-sm">
        <Outlet />
      </div>
    </main>
  );
}
