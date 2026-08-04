import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const TEACHER_PHONE = "0901000001";
const TEACHER_PASSWORD = "lan-password";

test("visiting a protected page unauthenticated redirects to login", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
});

test("rejected credentials show the server message on the form", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill("definitely-wrong-password");
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText("invalid phone or password")).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});

test("registers a new account and lands on the dashboard", async ({ page }) => {
  await page.goto("/register");
  await page.getByLabel("Họ và tên").fill("E2E Register");
  // Deterministic-enough 10-digit local number derived from the current
  // timestamp so parallel test runs don't collide on a duplicate phone.
  const phone = `09${String(Date.now()).slice(-8)}`;
  await page.getByLabel("Số điện thoại").fill(phone);
  await page.getByLabel("Mật khẩu").fill("e2e-password-123");
  await page.getByRole("button", { name: "Tạo tài khoản" }).click();
  await expect(page.getByText("Chào E2E Register")).toBeVisible();
});

test("logs in, keeps the session across a reload, and logs out", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào Cô Lan/)).toBeVisible();

  // The access token lives in memory only; a reload must restore the session
  // from the httpOnly refresh cookie without bouncing to /login.
  await page.reload();
  await expect(page.getByText(/Chào Cô Lan/)).toBeVisible();
  await expect(page).not.toHaveURL(/\/login/);

  await page.getByRole("button", { name: "Đăng xuất" }).click();
  await expect(page).toHaveURL(/\/login$/);

  // The refresh cookie is revoked: a reload must not resurrect the session.
  await page.reload();
  await expect(page).toHaveURL(/\/login$/);
});

test("a logged-in teacher with pending sessions sees the attendance alert on /", async ({
  page,
}) => {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();

  // The seeder leaves the two most recent past sessions unconfirmed
  // (`apps/api/seeds/seed.go`, `pendingAttendanceCount`).
  await expect(page.getByText("buổi đã qua chưa điểm danh")).toBeVisible();
  await expect(page.getByRole("link", { name: "Điểm danh ngay" }).first()).toBeVisible();
});
