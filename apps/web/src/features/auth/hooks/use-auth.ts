import { useMutation, useQueryClient } from "@tanstack/react-query";

import { login, logout, register } from "../api/auth-api";
import type { Session } from "../schemas/auth-schemas";
import { useAuthStore } from "../stores/auth-store";

function storeSession(session: Session): void {
  useAuthStore.getState().setSession(session.teacher, session.access_token);
}

export function useLogin() {
  return useMutation({ mutationFn: login, onSuccess: storeSession });
}

export function useRegister() {
  return useMutation({ mutationFn: register, onSuccess: storeSession });
}

export function useLogout() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: logout,
    // Even if revocation fails (network down, cookie already gone) the local
    // session ends: clear the store and drop every cached server response.
    onSettled: () => {
      useAuthStore.getState().clearSession();
      queryClient.clear();
    },
  });
}
