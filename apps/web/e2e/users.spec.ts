import { expect, test, type Page } from "@playwright/test";

const ADMIN_EMAIL = "admin@teka.local";
const ADMIN_PASSWORD = "admin-password";

async function loginAsAdmin(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(ADMIN_EMAIL);
  await page.getByLabel("Password").fill(ADMIN_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByText(/Welcome,/)).toBeVisible();
}

test("admin can browse, search, create, and delete users", async ({ page }) => {
  await loginAsAdmin(page);

  await page.getByRole("link", { name: "Users" }).click();
  await expect(page).toHaveURL(/\/users$/);
  await expect(page.getByRole("cell", { name: ADMIN_EMAIL, exact: true })).toBeVisible();

  // Search syncs into the URL and narrows the list.
  await page.getByLabel("Search users").fill("alice");
  await expect(page).toHaveURL(/q=alice/);
  await expect(page.getByRole("cell", { name: "alice@teka.local", exact: true })).toBeVisible();
  await expect(page.getByRole("cell", { name: "bob@teka.local", exact: true })).toBeHidden();
  await page.getByLabel("Search users").clear();

  // Create a user; default sort (-created_at) puts it on top of the list.
  const email = `e2e-user-${Date.now()}@example.com`;
  await page.getByRole("button", { name: "New user" }).click();
  await page.getByLabel("Name").fill("E2E Created");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill("e2e-password-123");
  await page.getByRole("button", { name: "Create user" }).click();
  await expect(page.getByRole("cell", { name: email, exact: true })).toBeVisible();

  // A duplicate email lands as an error under the email input, not a toast.
  await page.getByRole("button", { name: "New user" }).click();
  await page.getByLabel("Name").fill("Duplicate");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill("e2e-password-123");
  await page.getByRole("button", { name: "Create user" }).click();
  await expect(page.getByText("email already in use")).toBeVisible();
  await page.getByRole("button", { name: "Cancel" }).click();

  // Clean up: delete the created user through the row actions.
  await page.getByRole("button", { name: `Actions for ${email}` }).click();
  await page.getByRole("menuitem", { name: "Delete" }).click();
  await page.getByRole("button", { name: "Delete" }).click();
  await expect(page.getByRole("cell", { name: email, exact: true })).toBeHidden();
});

test("non-admin users see no admin affordances", async ({ page }) => {
  // Fresh registrations get the "user" role.
  await page.goto("/register");
  await page.getByLabel("Name").fill("E2E Regular");
  await page.getByLabel("Email").fill(`e2e-regular-${Date.now()}@example.com`);
  await page.getByLabel("Password").fill("e2e-password-123");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByText("Welcome, E2E Regular")).toBeVisible();

  await expect(page.getByRole("link", { name: "Users" })).toBeHidden();
});
