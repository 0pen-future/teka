import { apiClient } from "@/lib/api/client";
import { parseData, parseList, type Paginated } from "@/lib/api/envelope";

import { userSchema } from "../schemas/user-schemas";
import type { CreateUserInput, UpdateUserInput, User, UsersListParams } from "../types/user-types";

export async function getUsers(params: UsersListParams): Promise<Paginated<User>> {
  const res = await apiClient.get<unknown>("/users", { params });
  return parseList(userSchema, res.data);
}

export async function getUser(id: string): Promise<User> {
  const res = await apiClient.get<unknown>(`/users/${id}`);
  return parseData(userSchema, res.data);
}

export async function createUser(input: CreateUserInput): Promise<User> {
  const res = await apiClient.post<unknown>("/users", input);
  return parseData(userSchema, res.data);
}

export async function updateUser(id: string, input: Partial<UpdateUserInput>): Promise<User> {
  const res = await apiClient.patch<unknown>(`/users/${id}`, input);
  return parseData(userSchema, res.data);
}

export async function deleteUser(id: string): Promise<void> {
  await apiClient.delete(`/users/${id}`);
}
