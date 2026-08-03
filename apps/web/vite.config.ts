import path from "node:path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

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
  },
});
