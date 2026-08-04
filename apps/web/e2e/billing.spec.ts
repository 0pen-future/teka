import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const TEACHER_PHONE = "0901000001";
const TEACHER_PASSWORD = "lan-password";

async function login(page: import("@playwright/test").Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào Cô Lan/)).toBeVisible();
}

test("blocks close on a pending session, then closes once attendance is confirmed", async ({
  page,
}) => {
  await login(page);

  // `billing` (no period id) resolves to the current period.
  await page.goto("/billing");
  await expect(page).toHaveURL(/\/billing\/.+$/);

  const closeButton = page.getByRole("button", { name: /Chốt kỳ/ });
  const blockedPanel = page.getByText("Chưa thể chốt sổ");

  if (await blockedPanel.isVisible().catch(() => false)) {
    // Close is blocked while any past session in the period is unconfirmed
    // (R4 AC 1) — the button stays disabled and each offending session
    // links straight to its attendance screen.
    await expect(closeButton).toBeDisabled();
    await blockedPanel.locator("..").getByRole("link", { name: "Điểm danh" }).first().click();
    await expect(page).toHaveURL(/\/sessions\/.+\/attendance$/);

    const confirmButton = page.getByRole("button", { name: /vắng/ });
    await confirmButton.click();
    await expect(page).toHaveURL(/\/sessions$/);

    // Back on the review screen, the blocking panel clears once every past
    // session in the period is attended (may take another pass if more than
    // one session was blocking).
    await page.goto("/billing");
    await expect(page).toHaveURL(/\/billing\/.+$/);
  }

  await expect(blockedPanel).not.toBeVisible();
  await expect(closeButton).toBeEnabled();
  await closeButton.click();

  await page.getByRole("button", { name: "Chốt kỳ & tạo phiếu thu" }).click();

  // Closing is irreversible and locks the period — the footer bar swaps the
  // close action for the locked pill.
  await expect(page.getByText("✓ Đã chốt — kỳ đã khóa")).toBeVisible();

  // The locked status survives a reload, confirming the close persisted
  // server-side rather than only in client state.
  await page.reload();
  await expect(page.getByText("✓ Đã chốt — kỳ đã khóa")).toBeVisible();
});
