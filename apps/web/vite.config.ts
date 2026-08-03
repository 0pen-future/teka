import path from "node:path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Compose-only knobs, read at config-load time in Node (not VITE_-prefixed,
// so they never reach the client bundle). WEB_API_PROXY_TARGET makes /api
// same-origin in the browser; host-mode dev talks to localhost:8080 directly.
const proxyTarget = process.env.WEB_API_PROXY_TARGET;
const usePolling = process.env.WEB_USE_POLLING === "true";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    // Fixed port: the API's CORS allowlist and docs reference it. The default
    // "localhost" host keeps the printed URL on an allowlisted origin; note it
    // may bind only ::1 on macOS, so IPv4-only tools (curl 127.0.0.1) can miss
    // it — browsers fall back fine, and 127.0.0.1 is also CORS-allowlisted.
    port: 5173,
    strictPort: true,
    // Docker Desktop can drop inotify events across bind mounts; polling is
    // the documented fallback when HMR stops firing in the container.
    watch: usePolling ? { usePolling: true } : undefined,
    // No path rewrite: the API serves /api/v1/... and the refresh cookie is
    // scoped to /api/v1/auth, so the prefix must survive the hop.
    proxy: proxyTarget ? { "/api": { target: proxyTarget } } : undefined,
  },
});
