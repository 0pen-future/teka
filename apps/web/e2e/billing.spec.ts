import { expect, test, type Page } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const TEACHER_PHONE = "0901000001";
const TEACHER_PASSWORD = "lan-password";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();
}

/**
 * Opens the current period's review screen (`/billing` resolves the period)
 * and returns how many past sessions still block the close, read from the
 * pending-session gate response itself (`GET /sessions/pending?from=…` — the
 * `from` param distinguishes it from the dashboard's unscoped call). The
 * blocked panel and the close button's disabled state both derive from that
 * response, and polling the DOM instead would race the render: the button
 * mounts enabled while the request is in flight, and the panel appears one
 * paint after the response lands.
 */
async function openBillingPendingCount(page: Page): Promise<number> {
  const pendingResponse = page.waitForResponse(
    (response) => response.url().includes("/sessions/pending") && response.url().includes("from="),
  );
  await page.goto("/billing");
  await expect(page).toHaveURL(/\/billing\/.+$/);
  const body = (await (await pendingResponse).json()) as { data?: { items?: unknown[] } };
  return body.data?.items?.length ?? 0;
}

test("blocks close on a pending session, then closes once attendance is confirmed", async ({
  page,
}) => {
  await login(page);
  let pendingCount = await openBillingPendingCount(page);

  const closeButton = page.getByRole("button", { name: /Chốt kỳ/ });
  const blockedPanel = page.getByText("Chưa thể chốt sổ");

  // Close is blocked while any past session in the period is unconfirmed
  // (R4 AC 1) — the button stays disabled and each offending session links
  // straight to its attendance screen. The seeder leaves the most recent
  // past sessions unconfirmed (more than one), so clear them one
  // confirm-and-return pass at a time until none remain.
  expect(pendingCount).toBeGreaterThan(0);
  while (pendingCount > 0) {
    await expect(blockedPanel).toBeVisible();
    await expect(closeButton).toBeDisabled();
    await blockedPanel.locator("..").getByRole("link", { name: "Điểm danh" }).first().click();
    await expect(page).toHaveURL(/\/sessions\/.+\/attendance$/);

    await page.getByRole("button", { name: /vắng/ }).click();
    await expect(page).toHaveURL(/\/sessions$/);

    pendingCount = await openBillingPendingCount(page);
  }

  await expect(blockedPanel).not.toBeVisible();
  await expect(closeButton).toBeEnabled();
  await closeButton.click();

  await page.getByRole("dialog").getByRole("button", { name: "Chốt kỳ & tạo phiếu thu" }).click();

  // Closing is irreversible and locks the period — the footer bar swaps the
  // close action for the locked pill.
  await expect(page.getByText("✓ Đã chốt — kỳ đã khóa")).toBeVisible();

  // The locked status survives a reload, confirming the close persisted
  // server-side rather than only in client state.
  await page.reload();
  await expect(page.getByText("✓ Đã chốt — kỳ đã khóa")).toBeVisible();
});
