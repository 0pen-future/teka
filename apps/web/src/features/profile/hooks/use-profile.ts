import { useMutation } from "@tanstack/react-query";

import { useAuthStore } from "@/features/auth";

import { updateMe } from "../api/profile-api";

/**
 * Saves the profile and swaps the fresh teacher into the auth store so every
 * consumer of `user` (sidebar footer, dashboard greeting) updates at once.
 */
export function useUpdateMe() {
  return useMutation({
    mutationFn: updateMe,
    onSuccess: (teacher) => useAuthStore.getState().setUser(teacher),
  });
}
