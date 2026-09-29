import { useEffect, useRef, useState, type ReactNode } from "react";

import { Spinner } from "@/components/shared/spinner";

import { refreshSession } from "../api/auth-api";
import { hasSessionHint, useAuthStore } from "../stores/auth-store";

/** Pages a signed-out visitor lands on, where a refresh usually just 401s. */
const PUBLIC_AUTH_PATHS = ["/login", "/forgot-password", "/reset-password/", "/invite/"];

function isPublicAuthPath(pathname: string): boolean {
  return PUBLIC_AUTH_PATHS.some((path) =>
    path.endsWith("/") ? pathname.startsWith(path) : pathname === path,
  );
}

/**
 * On a full page load the in-memory access token is gone even when the
 * httpOnly refresh cookie is still valid. Attempt one silent refresh before
 * rendering the app so ProtectedRoute doesn't bounce a logged-in user to
 * /login. A 401 just means "no session to restore".
 *
 * On public auth pages the attempt runs only when this browser has signed in
 * before (`hasSessionHint`), so a first-time visitor sees no failed request.
 * Protected routes always try.
 */
export function SessionRestore({ children }: { children: ReactNode }) {
  const [restoring, setRestoring] = useState(
    () =>
      useAuthStore.getState().accessToken === null &&
      (hasSessionHint() || !isPublicAuthPath(window.location.pathname)),
  );
  const attempted = useRef(false);

  useEffect(() => {
    if (!restoring || attempted.current) {
      return;
    }
    attempted.current = true;
    refreshSession()
      .then((session) => useAuthStore.getState().setSession(session.teacher, session.access_token))
      .catch(() => undefined)
      .finally(() => setRestoring(false));
  }, [restoring]);

  if (restoring) {
    return (
      <div className="flex min-h-svh items-center justify-center">
        <Spinner className="size-6" />
      </div>
    );
  }
  return children;
}
