import path from "node:path";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  test: {
    // Playwright owns e2e/*.spec.ts; vitest runs only the src unit suite.
    include: ["src/**/*.test.{ts,tsx}"],
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    // Tests run fully offline: MSW intercepts every request, and env.ts needs
    // a valid URL to pass its boot validation. Nothing listens on this port.
    env: {
      VITE_API_URL: "http://localhost:8080/api/v1",
    },
  },
});
