import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const REGISTERED_PHONE = "0901000001";
const UNKNOWN_PHONE = "0909999999";

test("a registered phone shows the same generic confirmation as an unknown one", async ({
  page,
}) => {
  await page.goto("/forgot-password");
  await page.getByLabel("Số điện thoại").fill(REGISTERED_PHONE);
  await page.getByRole("button", { name: "Gửi liên kết đặt lại" }).click();

  await expect(page.getByText("Đã gửi yêu cầu")).toBeVisible();
  await expect(
    page.getByText("Nếu số điện thoại hợp lệ, liên kết đặt lại đã được gửi qua Zalo."),
  ).toBeVisible();
});

test("an unregistered phone shows the identical generic confirmation — no enumeration hint", async ({
  page,
}) => {
  await page.goto("/forgot-password");
  await page.getByLabel("Số điện thoại").fill(UNKNOWN_PHONE);
  await page.getByRole("button", { name: "Gửi liên kết đặt lại" }).click();

  await expect(page.getByText("Đã gửi yêu cầu")).toBeVisible();
  await expect(
    page.getByText("Nếu số điện thoại hợp lệ, liên kết đặt lại đã được gửi qua Zalo."),
  ).toBeVisible();
});

test("client-side phone validation blocks submission before it ever reaches the server", async ({
  page,
}) => {
  await page.goto("/forgot-password");
  await page.getByLabel("Số điện thoại").fill("12345");
  await page.getByRole("button", { name: "Gửi liên kết đặt lại" }).click();

  await expect(page.getByText("Số điện thoại không hợp lệ")).toBeVisible();
  await expect(page.getByText("Đã gửi yêu cầu")).toBeHidden();
});

test("an expired or already-used reset link shows the generic error, not a per-reason message", async ({
  page,
}) => {
  await page.goto("/reset-password/this-token-does-not-exist");
  await page.getByLabel("Mật khẩu mới").fill("brand-new-password");
  await page.getByLabel("Xác nhận mật khẩu").fill("brand-new-password");
  await page.getByRole("button", { name: "Đặt lại mật khẩu" }).click();

  await expect(
    page.getByText("Không thể đặt lại mật khẩu. Liên kết có thể đã hết hạn hoặc đã được dùng."),
  ).toBeVisible();
  await expect(page).toHaveURL(/\/reset-password\//);
});
