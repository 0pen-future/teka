import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
// The first seeded teacher owns the center; the second is a member of it.
const OWNER_PHONE = "0901000001";
const OWNER_PASSWORD = "lan-password";
const MEMBER_PHONE = "0901000002";
const MEMBER_PASSWORD = "minh-password";

async function login(
  page: import("@playwright/test").Page,
  phone: string,
  password: string,
  displayName: string,
) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(phone);
  await page.getByLabel("Mật khẩu").fill(password);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(
    page.getByText(new RegExp(`Chào buổi (sáng|trưa|chiều|tối), ${displayName}!`)),
  ).toBeVisible();
}

test("owner sees their mutation in the audit trail", async ({ page }) => {
  const suffix = Date.now();
  const contactName = `E2E Audit Phụ huynh ${suffix}`;
  const contactPhone = `09${String(suffix).slice(-8)}`;

  await login(page, OWNER_PHONE, OWNER_PASSWORD, "Cô Lan");

  // Two cheap real mutations for the trail to capture. The rename targets
  // /contacts/:id, so its audit row carries this run's contact id — that is
  // what proves the row below came from this run and not an earlier one on a
  // reused database.
  await page.goto("/contacts");
  await page.getByRole("button", { name: "Thêm người liên hệ" }).click();
  await page.getByLabel("Họ và tên").fill(contactName);
  await page.getByLabel("Số điện thoại").fill(contactPhone);
  await page.getByRole("button", { name: "Lưu" }).click();
  await expect(page.getByText(contactName)).toBeVisible();

  await page.getByText(contactName).click();
  await expect(page).toHaveURL(/\/contacts\/.+/);
  const contactId = new URL(page.url()).pathname.split("/").pop() ?? "";
  await page.getByRole("button", { name: "Sửa" }).click();
  await page.getByLabel("Họ và tên").fill(`${contactName} sửa`);
  await page.getByRole("button", { name: "Lưu" }).click();
  await expect(page.getByText(`${contactName} sửa`)).toBeVisible();

  // The audit entry is reachable from the nav for the owner.
  await page.getByRole("link", { name: "Nhật ký hoạt động" }).click();
  await expect(page).toHaveURL(/\/audit$/);
  await expect(page.getByRole("heading", { name: "Nhật ký hoạt động" })).toBeVisible();

  // Capture is async (batched flush), so the row may land a moment after the
  // mutation — reload until it shows instead of sleeping a fixed amount.
  await expect(async () => {
    await page.reload();
    await expect(page.getByRole("row").filter({ hasText: "contact.update" }).first()).toBeVisible({
      timeout: 2_000,
    });
  }).toPass({ timeout: 10_000 });

  // Rows are newest-first, so the first contact.update is this run's rename.
  const row = page.getByRole("row").filter({ hasText: "contact.update" }).first();
  await expect(row.getByText("Cô Lan", { exact: true })).toBeVisible();
  await expect(row.getByText("200", { exact: true })).toBeVisible();

  // The expanded request line carries the concrete path — including the id of
  // the contact created above, pinning the row to this run.
  await row.getByRole("button", { name: "Chi tiết contact.update" }).click();
  await expect(page.getByText(`PUT /api/v1/contacts/${contactId}`)).toBeVisible();

  // The create landed too.
  await expect(page.getByRole("row").filter({ hasText: "contact.create" }).first()).toBeVisible();
});

test("member gets no audit entry point and is redirected off /audit", async ({ page }) => {
  await login(page, MEMBER_PHONE, MEMBER_PASSWORD, "Thầy Minh");

  await expect(page.getByRole("link", { name: "Nhật ký hoạt động" })).toHaveCount(0);

  // Deep-linking is guarded too: members bounce back to the dashboard.
  await page.goto("/audit");
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("heading", { name: "Nhật ký hoạt động" })).toHaveCount(0);
});
