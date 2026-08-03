import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router";

import { useIsAuthenticated } from "@/features/auth/stores/auth-store";

/**
 * Gate for authenticated areas. Unauthenticated visitors are sent to /login
 * with the attempted location in state so login can return them there.
 */
export function ProtectedRoute({ children }: { children: ReactNode }) {
  const isAuthenticated = useIsAuthenticated();
  const location = useLocation();

  if (!isAuthenticated) {
    return (
      <Navigate
        to="/login"
        replace
        state={{ from: location.pathname + location.search + location.hash }}
      />
    );
  }
  return children;
}
