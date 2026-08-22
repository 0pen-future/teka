import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
// Cô Lan's roster seeds "Chị Hoa" with two children ("Bé An", "Bé Bình"),
// both enrolled in "Toán 8 - Tối Thứ Ba" — the two-child family this spec
// pays off in full.
const TEACHER_PHONE = "0901000001";
const TEACHER_PASSWORD = "lan-password";
const CONTACT_NAME = "Chị Hoa";
const CHILD_NAMES = ["Bé An", "Bé Bình"];
const CLASS_NAME = "Toán 8 - Tối Thứ Ba";

async function login(page: import("@playwright/test").Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();
}

/** The contact's card: the name paragraph's row-level ancestor (name -> name/phone wrapper -> button -> header -> card). */
function contactRow(page: import("@playwright/test").Page, name: string) {
  return page.getByText(name, { exact: true }).locator("xpath=../../../..");
}

function parseMoney(text: string): number {
  return Number.parseInt(text.replace(/[^\d]/g, ""), 10);
}

test("marks a two-child family paid in full, persists across reload, and reads paid in the by-class view", async ({
  page,
}) => {
  await login(page);

  // Resolve the current billing period id from the review screen, since
  // collections has no bare index route — it always needs a concrete period.
  await page.goto("/billing");
  await expect(page).toHaveURL(/\/billing\/.+$/);
  const periodId = page.url().split("/billing/")[1].split(/[/?]/)[0];

  await page.goto(`/collections/${periodId}`);
  await expect(page.getByRole("heading", { name: "Thu tiền" })).toBeVisible();

  const row = contactRow(page, CONTACT_NAME);
  await expect(row).toBeVisible();

  const outstandingText = await row
    .locator("span", { hasText: "Còn lại" })
    .locator("strong")
    .innerText();
  const outstanding = parseMoney(outstandingText);
  expect(outstanding).toBeGreaterThan(0);

  await row.getByRole("button", { name: "Thu tiền" }).click();

  const amountInput = page.getByLabel("Số tiền");
  await amountInput.click();
  await amountInput.fill(String(outstanding));
  await page.getByRole("button", { name: "Ghi nhận" }).click();

  // Paying exactly the outstanding total matches the server's default D8
  // allocation, so the footer collapses to the single "Xong" confirmation.
  await expect(page.getByText(/Đã ghi nhận/)).toBeVisible();
  await page.getByRole("button", { name: "Xong" }).click();

  await expect(row.locator("span", { hasText: "Còn lại" }).locator("strong")).toHaveText("0 ₫");
  await expect(row.getByText("Đã đóng")).toBeVisible();

  // The paid status persisted server-side, not only in client cache.
  await page.reload();
  const reloadedRow = contactRow(page, CONTACT_NAME);
  await expect(reloadedRow.getByText("Đã đóng")).toBeVisible();
  await expect(reloadedRow.locator("span", { hasText: "Còn lại" }).locator("strong")).toHaveText(
    "0 ₫",
  );

  // Switch to the by-class view — this contact's two children share one
  // class, so both must read paid there too (R7 AC 3).
  await page.getByRole("tab", { name: "Theo lớp" }).click();
  await page.getByRole("tab", { name: CLASS_NAME }).click();

  for (const childName of CHILD_NAMES) {
    const childRow = page.getByText(childName, { exact: true }).locator("xpath=../..");
    await expect(childRow.getByText("Đã đóng")).toBeVisible();
  }
});
