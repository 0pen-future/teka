import { useEffect, useRef, useState, type ReactNode } from "react";

import { Spinner } from "@/components/shared/spinner";

import { refreshSession } from "../api/auth-api";
import { useAuthStore } from "../stores/auth-store";

/**
 * On a full page load the in-memory access token is gone even when the
 * httpOnly refresh cookie is still valid. Attempt one silent refresh before
 * rendering the app so ProtectedRoute doesn't bounce a logged-in user to
 * /login. A 401 just means "no session to restore".
 */
export function SessionRestore({ children }: { children: ReactNode }) {
  const [restoring, setRestoring] = useState(() => useAuthStore.getState().accessToken === null);
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
