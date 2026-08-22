import { useMutation, useQueryClient } from "@tanstack/react-query";

import { forgotPassword, login, logout, resetPassword } from "../api/auth-api";
import type { Session } from "../schemas/auth-schemas";
import { useAuthStore } from "../stores/auth-store";

function storeSession(session: Session): void {
  useAuthStore.getState().setSession(session.teacher, session.access_token);
}

export function useLogin() {
  return useMutation({ mutationFn: login, onSuccess: storeSession });
}

export function useForgotPassword() {
  return useMutation({ mutationFn: forgotPassword });
}

/**
 * A reset does not sign the teacher in — it only redeems the token and
 * revokes every existing refresh token, so the page sends them to `/login`
 * with their new password rather than establishing a session here.
 */
export function useResetPassword() {
  return useMutation({ mutationFn: resetPassword });
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
