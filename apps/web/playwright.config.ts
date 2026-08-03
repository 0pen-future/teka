import { defineConfig } from "@playwright/test";

/**
 * E2E suite against a running stack (`make dev`, or API + `npm run dev`
 * locally). Specs authenticate with the seeded dev credentials from
 * `apps/api` (`make seed`), so run the seeder first on a fresh database.
 */
export default defineConfig({
  testDir: "./e2e",
  // Specs mutate the shared users table; keep them sequential.
  fullyParallel: false,
  workers: 1,
  timeout: 30_000,
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5173",
    trace: "retain-on-failure",
  },
});
