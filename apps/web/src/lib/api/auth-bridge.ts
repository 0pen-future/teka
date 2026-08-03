/**
 * Session hooks the interceptors call without importing feature code. The auth
 * feature registers its store here (feature -> lib), so lib never depends on
 * features and Phase 6's auth API hooks cannot create an import cycle.
 */
export interface AuthBridge {
  getAccessToken(): string | null;
  setAccessToken(token: string): void;
  clearSession(): void;
}

let bridge: AuthBridge | null = null;

export function connectAuthBridge(next: AuthBridge): void {
  bridge = next;
}

export function getAuthBridge(): AuthBridge | null {
  return bridge;
}

/**
 * After a refresh fails the cookie is gone or revoked; retrying /auth/refresh
 * on every subsequent 401 would only hammer a dead endpoint. The gate stays
 * closed until a new login stores a fresh session.
 */
let refreshDead = false;

export function markRefreshDead(): void {
  refreshDead = true;
}

export function markRefreshAlive(): void {
  refreshDead = false;
}

export function isRefreshDead(): boolean {
  return refreshDead;
}
