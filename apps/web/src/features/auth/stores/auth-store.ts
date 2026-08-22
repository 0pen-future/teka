import { create } from "zustand";

import { connectAuthBridge, markRefreshAlive } from "@/lib/api/auth-bridge";

import type { Teacher } from "../schemas/auth-schemas";

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
    set({ user, accessToken });
  },
  setUser: (user) => set({ user }),
  setAccessToken: (accessToken) => set({ accessToken }),
  clearSession: () => set({ user: null, accessToken: null }),
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
