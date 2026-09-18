import { expect, type APIRequestContext, type Page } from "@playwright/test";

/** Dev-only credentials created by the API seeder (`apps/api` seed command). */
export const OWNER_PHONE = "0901000001";
export const OWNER_PASSWORD = "lan-password";

/** The seeded center's other teacher (a plain member, no `tasks.view_all`/`tasks.manage_board`). */
export const MEMBER_PHONE = "0901000002";
export const MEMBER_PASSWORD = "minh-password";

/** Signs the seeded owner in through the UI and waits for the dashboard greeting. */
export async function loginAsOwner(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(OWNER_PHONE);
  await page.getByLabel("Mật khẩu").fill(OWNER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();
}

/** Signs the seeded member teacher (Thầy Minh) in through the UI. */
export async function loginAsMember(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(MEMBER_PHONE);
  await page.getByLabel("Mật khẩu").fill(MEMBER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Thầy Minh!/)).toBeVisible();
}

/**
 * Logs in over the API (the web dev server proxies `/api` to the API container)
 * and returns the bearer token, so a spec can talk to the API directly for
 * setup or for payloads the UI would never send.
 */
export async function apiLoginAsOwner(request: APIRequestContext): Promise<string> {
  const response = await request.post("/api/v1/auth/login", {
    data: { phone: OWNER_PHONE, password: OWNER_PASSWORD },
  });
  expect(response.ok(), `login failed: ${response.status()}`).toBe(true);
  const body = (await response.json()) as { data: { access_token: string } };
  return body.data.access_token;
}
