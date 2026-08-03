import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { createUser, deleteUser, getUser, getUsers, updateUser } from "../api/users-api";
import type { CreateUserInput, UpdateUserInput, UsersListParams } from "../types/user-types";

export const usersKeys = {
  all: ["users"] as const,
  lists: () => [...usersKeys.all, "list"] as const,
  list: (params: UsersListParams) => [...usersKeys.lists(), params] as const,
  details: () => [...usersKeys.all, "detail"] as const,
  detail: (id: string) => [...usersKeys.details(), id] as const,
};

export function useUsersList(params: UsersListParams) {
  return useQuery({
    queryKey: usersKeys.list(params),
    queryFn: () => getUsers(params),
    // Keep the previous page on screen while the next one loads.
    placeholderData: keepPreviousData,
  });
}

export function useUser(id: string) {
  return useQuery({
    queryKey: usersKeys.detail(id),
    queryFn: () => getUser(id),
  });
}

export function useCreateUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateUserInput) => createUser(input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: usersKeys.lists() }),
  });
}

export function useUpdateUser(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: Partial<UpdateUserInput>) => updateUser(id, input),
    onSuccess: (user) => {
      queryClient.setQueryData(usersKeys.detail(id), user);
      void queryClient.invalidateQueries({ queryKey: usersKeys.lists() });
    },
  });
}

export function useDeleteUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteUser(id),
    onSuccess: (_data, id) => {
      queryClient.removeQueries({ queryKey: usersKeys.detail(id) });
      void queryClient.invalidateQueries({ queryKey: usersKeys.lists() });
    },
  });
}
