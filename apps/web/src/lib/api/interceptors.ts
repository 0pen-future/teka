import type { AxiosInstance, InternalAxiosRequestConfig } from "axios";
import { isAxiosError } from "axios";

import { getAuthBridge, isRefreshDead, markRefreshAlive, markRefreshDead } from "./auth-bridge";
import { toApiError } from "./errors";

interface RetriableConfig extends InternalAxiosRequestConfig {
  _retried?: boolean;
}

interface RefreshEnvelope {
  data: {
    access_token: string;
  };
}

/**
 * Refreshing must be single-flight: when several requests 401 at once, only
 * one POST /auth/refresh goes out (token rotation would revoke the family if
 * the same refresh token were replayed) and the rest await its result.
 */
let refreshInFlight: Promise<string | null> | null = null;

function refreshAccessToken(client: AxiosInstance): Promise<string | null> {
  refreshInFlight ??= client
    .post<RefreshEnvelope>("/auth/refresh", null, { _retried: true } as Partial<RetriableConfig>)
    .then((res) => {
      const token = res.data.data.access_token;
      getAuthBridge()?.setAccessToken(token);
      markRefreshAlive();
      return token;
    })
    .catch(() => {
      // Session over: close the refresh gate and clear the store;
      // ProtectedRoute redirects to /login.
      markRefreshDead();
      getAuthBridge()?.clearSession();
      return null;
    })
    .finally(() => {
      refreshInFlight = null;
    });
  return refreshInFlight;
}

export function setupInterceptors(client: AxiosInstance): void {
  client.interceptors.request.use((config) => {
    const token = getAuthBridge()?.getAccessToken();
    if (token && !config.headers.Authorization) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  });

  client.interceptors.response.use(
    (response) => response,
    async (error: unknown) => {
      if (isAxiosError(error) && error.response?.status === 401 && error.config) {
        const config = error.config as RetriableConfig;
        const isAuthCall = config.url?.startsWith("/auth/") ?? false;
        if (!config._retried && !isAuthCall) {
          const currentToken = getAuthBridge()?.getAccessToken() ?? null;
          const sentAuth = config.headers.Authorization;
          if (currentToken && sentAuth !== `Bearer ${currentToken}`) {
            // The store already holds a newer token than this request carried
            // (a refresh settled while it was in flight) — retry with it
            // instead of firing another refresh.
            config._retried = true;
            config.headers.Authorization = `Bearer ${currentToken}`;
            return client(config);
          }
          if (!isRefreshDead()) {
            config._retried = true;
            const token = await refreshAccessToken(client);
            if (token) {
              config.headers.Authorization = `Bearer ${token}`;
              return client(config);
            }
          }
        }
      }
      throw toApiError(error);
    },
  );
}
