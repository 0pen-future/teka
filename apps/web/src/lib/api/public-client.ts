import axios from "axios";

import { env } from "@/lib/config/env";

import { toApiError } from "./errors";

/**
 * A dedicated axios instance for the public, unauthenticated parent
 * statement route (`/s/:token`). It deliberately carries no auth
 * interceptors (`./interceptors.ts`) and no credentials: on this route a
 * 401/403/404 is a normal, expected outcome — an unknown, expired, or
 * already-paid token — not a signal to attempt a refresh or redirect to
 * `/login`. Reusing `apiClient` here would pull the refresh-and-redirect
 * dance in for a visitor who was never logged in. Do not "fix" this by
 * calling `setupInterceptors` on this instance.
 *
 * Errors are still normalized to `ApiError` via `toApiError` so feature code
 * has one consistent shape to branch on — that normalization is auth-agnostic
 * and safe to reuse as-is.
 */
/**
 * The public statement routes are deliberately mounted at the server root
 * (`/public/statements`, see `apps/api/internal/features/statements/routes.go`),
 * outside the versioned `/api/v1` group — so the base URL is `VITE_API_URL`
 * with its `/api/v1` suffix stripped: a full origin in host-mode dev
 * (`http://localhost:8080`) or the empty string under the compose/Vite proxy
 * (same-origin, proxied via the `/public` rule in `vite.config.ts`).
 */
const publicBaseURL = env.VITE_API_URL.replace(/\/api\/v1\/?$/, "");

export const publicApiClient = axios.create({
  baseURL: publicBaseURL,
  withCredentials: false,
  timeout: 10_000,
});

publicApiClient.interceptors.response.use(
  (response) => response,
  (error: unknown) => Promise.reject(toApiError(error)),
);
