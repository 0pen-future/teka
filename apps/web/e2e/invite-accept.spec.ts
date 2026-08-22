import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const OWNER_PHONE = "0901000001";
const OWNER_PASSWORD = "lan-password";

async function loginAsOwner(page: import("@playwright/test").Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(OWNER_PHONE);
  await page.getByLabel("Mật khẩu").fill(OWNER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();
}

test("owner invites a teacher, who accepts the link and logs in", async ({ page, context }) => {
  // Timestamp suffix keeps the invitee phone unique across reruns against
  // one seeded stack (`playwright.config.ts` runs specs sequentially).
  const suffix = Date.now();
  const inviteePhone = `09${String(suffix).slice(-8)}`;
  const inviteeName = `E2E Giáo Viên ${suffix}`;
  const inviteePassword = "e2e-invitee-password";

  await loginAsOwner(page);
  await page.goto("/center");

  await page.getByLabel("Số điện thoại").fill(inviteePhone);
  await page.getByRole("button", { name: "Gửi lời mời" }).click();

  await expect(page.getByRole("heading", { name: "Đã tạo lời mời" })).toBeVisible();
  const link = await page.getByLabel("Liên kết mời").inputValue();
  expect(link).toContain("/invite/");
  // `exact: true` targets the footer button; the modal's built-in close
  // button is named "Đóng hộp thoại" so the two stay distinguishable.
  await page.getByRole("button", { name: "Đóng", exact: true }).click();

  // The invite creation invalidates the pending-invite list, so the new
  // invitee shows up there right away.
  await expect(page.getByText(inviteePhone)).toBeVisible();

  // The invited teacher opens the link from a fresh, logged-out context.
  const invitePath = new URL(link).pathname;
  const inviteePage = await context.browser()!.newPage();
  await inviteePage.goto(invitePath);

  await expect(inviteePage.getByText("Trung Tâm Bình Minh")).toBeVisible();
  await inviteePage.getByLabel("Họ và tên").fill(inviteeName);
  await inviteePage.getByLabel("Mật khẩu", { exact: true }).fill(inviteePassword);
  await inviteePage.getByLabel("Xác nhận mật khẩu").fill(inviteePassword);
  await inviteePage.getByRole("button", { name: "Tạo tài khoản" }).click();

  await expect(inviteePage).toHaveURL(/\/login$/);

  await inviteePage.getByLabel("Số điện thoại").fill(inviteePhone);
  await inviteePage.getByLabel("Mật khẩu").fill(inviteePassword);
  await inviteePage.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(
    inviteePage.getByText(new RegExp(`Chào buổi (sáng|trưa|chiều|tối), ${inviteeName}!`)),
  ).toBeVisible();

  await inviteePage.close();
});

test("an unknown invite token shows the generic error, not a per-reason message", async ({
  page,
}) => {
  await page.goto("/invite/this-token-does-not-exist");
  await expect(page.getByText(/không hợp lệ|hết hạn|không tồn tại/i)).toBeVisible();
});
