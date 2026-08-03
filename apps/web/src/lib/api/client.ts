import axios from "axios";

import { env } from "@/lib/config/env";

import { setupInterceptors } from "./interceptors";

/**
 * The single axios instance for the app. withCredentials carries the httpOnly
 * refresh cookie; the access token itself lives only in the auth store and is
 * attached per-request by the interceptors.
 */
export const apiClient = axios.create({
  baseURL: env.VITE_API_URL,
  withCredentials: true,
  timeout: 10_000,
});

setupInterceptors(apiClient);
