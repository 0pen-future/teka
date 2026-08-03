// Public surface of the auth feature. The app shell and other features import
// ONLY from here; routes.tsx stays a separate entry so the router can mount
// pages without pulling them into every consumer's chunk.
export { ProtectedRoute } from "./components/protected-route";
export { SessionRestore } from "./components/session-restore";
export { useLogout } from "./hooks/use-auth";
export { useAuthStore, useIsAuthenticated } from "./stores/auth-store";
