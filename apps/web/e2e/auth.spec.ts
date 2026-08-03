import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const ADMIN_EMAIL = "admin@teka.local";
const ADMIN_PASSWORD = "admin-password";

test("visiting a protected page unauthenticated redirects to login", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
});

test("rejected credentials show the server message on the form", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Email").fill(ADMIN_EMAIL);
  await page.getByLabel("Password").fill("definitely-wrong-password");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByText("invalid email or password")).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});

test("registers a new account and lands on the dashboard", async ({ page }) => {
  await page.goto("/register");
  await page.getByLabel("Name").fill("E2E Register");
  await page.getByLabel("Email").fill(`e2e-reg-${Date.now()}@example.com`);
  await page.getByLabel("Password").fill("e2e-password-123");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByText("Welcome, E2E Register")).toBeVisible();
});

test("logs in, keeps the session across a reload, and logs out", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Email").fill(ADMIN_EMAIL);
  await page.getByLabel("Password").fill(ADMIN_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByText(/Welcome,/)).toBeVisible();

  // The access token lives in memory only; a reload must restore the session
  // from the httpOnly refresh cookie without bouncing to /login.
  await page.reload();
  await expect(page.getByText(/Welcome,/)).toBeVisible();
  await expect(page).not.toHaveURL(/\/login/);

  await page.getByRole("button", { name: "Account menu" }).click();
  await page.getByRole("menuitem", { name: "Log out" }).click();
  await expect(page).toHaveURL(/\/login$/);

  // The refresh cookie is revoked: a reload must not resurrect the session.
  await page.reload();
  await expect(page).toHaveURL(/\/login$/);
});
