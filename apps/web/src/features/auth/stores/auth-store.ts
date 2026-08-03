import { create } from "zustand";

import { connectAuthBridge, markRefreshAlive } from "@/lib/api/auth-bridge";

export interface AuthUser {
  id: string;
  email: string;
  name: string;
  role: "admin" | "user";
}

interface AuthState {
  /** Access token lives in memory only — never localStorage (XSS surface). */
  accessToken: string | null;
  user: AuthUser | null;
  setSession: (user: AuthUser, accessToken: string) => void;
  setAccessToken: (accessToken: string) => void;
  clearSession: () => void;
}

/**
 * Session skeleton: state and actions only. Phase 6 wires the real
 * login/register/me flows on top of these actions.
 */
export const useAuthStore = create<AuthState>()((set) => ({
  accessToken: null,
  user: null,
  setSession: (user, accessToken) => {
    // A fresh login re-opens the interceptors' refresh gate after a dead session.
    markRefreshAlive();
    set({ user, accessToken });
  },
  setAccessToken: (accessToken) => set({ accessToken }),
  clearSession: () => set({ user: null, accessToken: null }),
}));

// Register the store with the API layer. lib/api never imports feature code;
// it reaches the session only through this bridge.
connectAuthBridge({
  getAccessToken: () => useAuthStore.getState().accessToken,
  setAccessToken: (token) => useAuthStore.getState().setAccessToken(token),
  clearSession: () => useAuthStore.getState().clearSession(),
});

export function useIsAuthenticated(): boolean {
  return useAuthStore((state) => state.accessToken !== null);
}
