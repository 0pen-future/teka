import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  forgotPasswordResponseSchema,
  sessionSchema,
  teacherSchema,
  type ForgotPasswordInput,
  type LoginInput,
  type ResetPasswordInput,
  type Session,
  type Teacher,
} from "../schemas/auth-schemas";

export async function login(input: LoginInput): Promise<Session> {
  const res = await apiClient.post<unknown>("/auth/login", input);
  return parseData(sessionSchema, res.data);
}

/** Always resolves — the API returns the same generic body for every phone. */
export async function forgotPassword(input: ForgotPasswordInput): Promise<void> {
  const res = await apiClient.post<unknown>("/auth/forgot-password", input);
  parseData(forgotPasswordResponseSchema, res.data);
}

export async function resetPassword(input: ResetPasswordInput): Promise<void> {
  await apiClient.post("/auth/reset-password", input);
}

/**
 * Exchange the httpOnly refresh cookie for a fresh session. 401 simply means
 * "no session to restore" for a visitor without a cookie.
 */
export async function refreshSession(): Promise<Session> {
  const res = await apiClient.post<unknown>("/auth/refresh");
  return parseData(sessionSchema, res.data);
}

/** Revokes the refresh-token family; idempotent server-side. */
export async function logout(): Promise<void> {
  await apiClient.post("/auth/logout");
}

/** Fetches the authenticated teacher's profile. */
export async function getMe(): Promise<Teacher> {
  const res = await apiClient.get<unknown>("/me");
  return parseData(teacherSchema, res.data);
}
