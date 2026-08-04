import { useEffect, useRef, useState, type ReactNode } from "react";

import { Spinner } from "@/components/shared/spinner";

import { refreshSession } from "../api/auth-api";
import { useAuthStore } from "../stores/auth-store";

// Hardcoded rather than imported from "@/features/statement": that feature's
// isolation boundary forbids it from depending on anything auth-related, so
// the dependency cannot run the other way either. Keep this in sync with
// STATEMENT_PATH_PREFIX in apps/web/src/features/statement/routes.tsx.
const PUBLIC_STATEMENT_PATH_PREFIX = "/s/";

/**
 * On a full page load the in-memory access token is gone even when the
 * httpOnly refresh cookie is still valid. Attempt one silent refresh before
 * rendering the app so ProtectedRoute doesn't bounce a logged-in user to
 * /login. A 401 just means "no session to restore".
 *
 * On the public parent-statement route this attempt is skipped entirely: a
 * parent visiting `/s/:token` never has a teacher session to restore, and
 * firing `/auth/refresh` there is a pointless request that also 401s for
 * every parent.
 */
export function SessionRestore({ children }: { children: ReactNode }) {
  const isPublicStatementRoute = window.location.pathname.startsWith(PUBLIC_STATEMENT_PATH_PREFIX);
  const [restoring, setRestoring] = useState(
    () => !isPublicStatementRoute && useAuthStore.getState().accessToken === null,
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
