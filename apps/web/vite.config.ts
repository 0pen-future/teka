import path from "node:path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { visualizer } from "rollup-plugin-visualizer";
import { defineConfig, type PluginOption } from "vite";

// Compose-only knobs, read at config-load time in Node (not VITE_-prefixed,
// so they never reach the client bundle). WEB_API_PROXY_TARGET makes /api
// same-origin in the browser; host-mode dev talks to localhost:8080 directly.
const proxyTarget = process.env.WEB_API_PROXY_TARGET;
const usePolling = process.env.WEB_USE_POLLING === "true";
// `npm run build:analyze` writes a bundle treemap to stats.html (gitignored).
const analyze = process.env.ANALYZE === "true";

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    // Cast: the plugin declares rollup's Plugin type, which vite's rolldown
    // types don't recognize; the hook shape is compatible.
    ...(analyze ? [visualizer({ filename: "stats.html", gzipSize: true }) as PluginOption] : []),
  ],
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
    // scoped to /api/v1/auth, so the prefix must survive the hop. /public is
    // the unauthenticated parent-statement group, mounted at the API's root
    // (outside /api/v1) — it must be proxied too or the SPA fallback would
    // answer those JSON requests with index.html.
    proxy: proxyTarget
      ? { "/api": { target: proxyTarget }, "/public": { target: proxyTarget } }
      : undefined,
  },
});
