import { expect, test, type Page } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
// Cô Thu is hoc_vu on "Toán 8 - Tối Thứ Ba", whose open enrollments are Bé An
// and Bé Bình (Bé Cường's ended in March).
const STAFF_CLASS = "Toán 8 - Tối Thứ Ba";
const HOC_VU = { phone: "0901000003", password: "thu-password", name: "Cô Thu" };

test.use({ viewport: { width: 375, height: 812 } });

async function login(page: Page, user: { phone: string; password: string; name: string }) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(user.phone);
  await page.getByLabel("Mật khẩu").fill(user.password);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(
    page.getByText(new RegExp(`Chào buổi (sáng|trưa|chiều|tối), ${user.name}!`)),
  ).toBeVisible();
}

test("phone records: class sheet, live search, no-match state and detail link", async ({
  page,
}) => {
  await login(page, HOC_VU);
  await page.goto("/records");

  // Below `sm` the class picker opens as a bottom sheet.
  await page.getByRole("button", { name: /^Lớp/ }).click();
  await expect(page.getByRole("dialog", { name: "Chọn lớp" })).toBeVisible();
  await page.getByRole("option", { name: new RegExp(STAFF_CLASS) }).click();
  await expect(page.getByRole("dialog", { name: "Chọn lớp" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^Lớp/ })).toHaveAccessibleName(
    new RegExp(`^Lớp ${STAFF_CLASS} · 2 HS$`),
  );
  await expect(page.getByText("Bé An", { exact: true })).toBeVisible();
  await expect(page.getByText("Bé Bình", { exact: true })).toBeVisible();

  // Diacritic-insensitive live filter with a highlighted match and counter.
  const search = page.getByLabel("Tìm học sinh");
  await search.fill("an");
  await expect(page).toHaveURL(/[?&]q=an/);
  await expect(page.getByText("Bé An", { exact: true })).toBeVisible();
  await expect(page.getByText("Bé Bình", { exact: true })).toHaveCount(0);
  await expect(page.locator("mark")).toHaveText("An");
  await expect(page.getByRole("status")).toHaveText("1 / 2 học sinh");

  // No match: in-card empty state whose button clears the search.
  await search.fill("zzz");
  const emptyState = page.getByText("Không tìm thấy học sinh nào khớp “zzz”").locator("..");
  await expect(emptyState).toBeVisible();
  await emptyState.getByRole("button", { name: "Xoá tìm kiếm" }).click();
  await expect(search).toHaveValue("");
  await expect(page).not.toHaveURL(/[?&]q=/);
  await expect(page.getByText("Bé Bình", { exact: true })).toBeVisible();

  // The two-line rows never push the page wider than the phone viewport.
  const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
  expect(scrollWidth).toBeLessThanOrEqual(375);

  // Compact rows keep the "Xem hồ sơ" accessible name on the short "Xem" button.
  await page
    .getByText("Bé An", { exact: true })
    .locator("..")
    .getByRole("button", { name: "Xem hồ sơ" })
    .click();
  await expect(page).toHaveURL(/\/records\/[0-9a-f-]+$/);
  await expect(page.getByRole("heading", { name: "Bé An" })).toBeVisible();
});
