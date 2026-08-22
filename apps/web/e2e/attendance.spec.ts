import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const TEACHER_PHONE = "0901000001";
const TEACHER_PASSWORD = "lan-password";

async function login(page: import("@playwright/test").Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();
}

test("marks absentees and confirms a pending session in one touch each, clearing the dashboard alert", async ({
  page,
}) => {
  await login(page);

  // The seeder leaves the two most recent past sessions unconfirmed
  // (`apps/api/seeds/seed.go`, `pendingAttendanceCount`) — jump straight into
  // one from the dashboard's pending-attendance alert rather than hand-rolling
  // a session first.
  await expect(page.getByText("buổi đã dạy nhưng chưa điểm danh")).toBeVisible();
  await page.getByRole("link", { name: "Điểm danh ngay" }).first().click();
  await expect(page).toHaveURL(/\/sessions\/.+\/attendance$/);

  // Everyone renders present by default — the roster loads before any tap.
  // Each attendance row is a tappable button with `aria-pressed`, distinct
  // from the confirm bar and other page buttons which carry no such attribute.
  const rows = page.locator("button[aria-pressed]");
  await expect(rows.first()).toBeVisible();
  const rowCount = await rows.count();
  expect(rowCount).toBeGreaterThan(0);

  // Interaction 1 and 2: tap the first two students absent — purely local
  // state, no confirmation dialog, no per-row network round trip to wait on.
  await rows.nth(0).click();
  await expect(rows.nth(0)).toHaveAttribute("aria-pressed", "true");
  if (rowCount > 1) {
    await rows.nth(1).click();
    await expect(rows.nth(1)).toHaveAttribute("aria-pressed", "true");
  }
  const absentCount = rowCount > 1 ? 2 : 1;

  // Interaction 3: the single confirm tap writes the whole roster at once.
  const confirmButton = page.getByRole("button", { name: /vắng/ });
  await expect(confirmButton).toHaveText(new RegExp(`${absentCount} vắng`));
  await confirmButton.click();

  // Confirming this session navigates back to the list and drops it from
  // "cần điểm danh" — one fewer pending session than before.
  await expect(page).toHaveURL(/\/sessions$/);

  // The dashboard's pending count reflects the just-confirmed session.
  await page.goto("/");
  const pendingBanner = page.getByText(/buổi đã dạy nhưng chưa điểm danh/);
  if (await pendingBanner.isVisible().catch(() => false)) {
    await expect(pendingBanner).toContainText("1 buổi đã dạy nhưng chưa điểm danh");
  } else {
    await expect(pendingBanner).not.toBeVisible();
  }
});

test("cancelling a session takes a reason and bills nobody", async ({ page }) => {
  await login(page);

  // Cancel an upcoming session rather than a pending one: the pending feed is
  // shared suite state the previous test already drained, while the weekly
  // schedule always has future sessions. Extending the list's end date two
  // weeks out guarantees at least one "Sắp diễn ra" row regardless of which
  // weekday the suite runs on.
  await page.goto("/sessions");
  const to = new Date();
  to.setDate(to.getDate() + 14);
  await page.getByLabel("Đến").fill(to.toISOString().slice(0, 10));

  const upcoming = page.getByRole("link").filter({ hasText: "Sắp diễn ra" }).first();
  await expect(upcoming).toBeVisible();
  await upcoming.click();
  await expect(page).toHaveURL(/\/sessions\/.+\/attendance$/);
  const sessionUrl = page.url();

  await page.getByRole("button", { name: "Huỷ buổi học" }).click();
  await page.getByLabel("Lý do huỷ").fill("Nghỉ lễ Quốc khánh");
  await page.getByRole("button", { name: "Xác nhận huỷ" }).click();

  // Cancelling navigates back to the session list; the cancelled state lives
  // on the session page itself, so revisit the saved URL to assert it.
  await expect(page).toHaveURL(/\/sessions$/);
  await page.goto(sessionUrl);
  await expect(page.getByText("Buổi học đã huỷ")).toBeVisible();
  await expect(page.getByText("Nghỉ lễ Quốc khánh")).toBeVisible();
  await expect(page.getByText("Không tính tiền cho học sinh nào.")).toBeVisible();
});
