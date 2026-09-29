import { create } from "zustand";

import { connectAuthBridge, markRefreshAlive } from "@/lib/api/auth-bridge";

import type { Teacher } from "../schemas/auth-schemas";

/**
 * Non-secret hint that this browser has signed in before. Lets public auth
 * pages (login, forgot/reset password, invite) skip a refresh attempt that
 * would only 401 for a first-time visitor. Holds "1", never a token.
 */
const SESSION_HINT_KEY = "teka.hasSession";

function writeSessionHint(present: boolean) {
  try {
    if (present) window.localStorage.setItem(SESSION_HINT_KEY, "1");
    else window.localStorage.removeItem(SESSION_HINT_KEY);
  } catch {
    // Storage can be unavailable (private mode, blocked site data); the hint
    // is an optimisation, so losing it only costs one extra refresh call.
  }
}

export function hasSessionHint(): boolean {
  try {
    return window.localStorage.getItem(SESSION_HINT_KEY) === "1";
  } catch {
    return false;
  }
}

interface AuthState {
  /** Access token lives in memory only — never localStorage (XSS surface). */
  accessToken: string | null;
  user: Teacher | null;
  setSession: (user: Teacher, accessToken: string) => void;
  /** Replaces the cached profile after PUT /me without touching the token. */
  setUser: (user: Teacher) => void;
  setAccessToken: (accessToken: string) => void;
  clearSession: () => void;
}

export const useAuthStore = create<AuthState>()((set) => ({
  accessToken: null,
  user: null,
  setSession: (user, accessToken) => {
    // A fresh login re-opens the interceptors' refresh gate after a dead session.
    markRefreshAlive();
    writeSessionHint(true);
    set({ user, accessToken });
  },
  setUser: (user) => set({ user }),
  setAccessToken: (accessToken) => set({ accessToken }),
  clearSession: () => {
    writeSessionHint(false);
    set({ user: null, accessToken: null });
  },
}));

export function useIsAuthenticated(): boolean {
  return useAuthStore((state) => state.accessToken !== null);
}

// Register the store with the API layer. lib/api never imports feature code;
// it reaches the session only through this bridge.
connectAuthBridge({
  getAccessToken: () => useAuthStore.getState().accessToken,
  setAccessToken: (token) => useAuthStore.getState().setAccessToken(token),
  clearSession: () => useAuthStore.getState().clearSession(),
});
