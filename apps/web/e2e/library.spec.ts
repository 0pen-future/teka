import { expect, test, type Page } from "@playwright/test";

import { loginAsOwner } from "./helpers/auth.js";

// Dev-only seed: Cô Lan owns the center, so she holds every library key. The
// template code carries a per-run suffix, which keeps the journey clear of the
// duplicate-code 409 on a reused database, and the afterEach deletes whatever
// the run created so the catalog stays as the seed left it.
const RUN_CODE = `E2E-${Date.now().toString(36).toUpperCase()}`.slice(0, 20);
const TEMPLATE_NAME = `Chương trình e2e ${RUN_CODE}`;

/** Deletes the template this run created, if it is still in the catalog. */
async function deleteRunTemplate(page: Page) {
  await page.goto("/library");
  await page.getByRole("searchbox", { name: "Tìm chương trình mẫu" }).fill(RUN_CODE);
  const row = page.getByRole("row").filter({ hasText: RUN_CODE });
  const noMatch = page.getByText("Không có chương trình nào khớp từ khoá.");
  await expect(row.or(noMatch)).toBeVisible();
  if ((await row.count()) === 0) return;

  await row.getByRole("link").first().click();
  await page.getByRole("button", { name: "Xoá chương trình" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Xoá chương trình" }).click();
  await expect(page.getByText(`Đã xoá chương trình ${RUN_CODE}`)).toBeVisible();
}

test.afterEach(async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await loginAsOwner(page);
    await deleteRunTemplate(page);
  } finally {
    await context.close();
  }
});

test("owner drafts a template with two lessons, reorders them and publishes", async ({ page }) => {
  await loginAsOwner(page);

  // The nav entry lives in the Giảng dạy group.
  await page.goto("/library");
  await expect(page.getByRole("heading", { name: "Kho học liệu" })).toBeVisible();

  await page.getByRole("button", { name: "Tạo chương trình mẫu" }).click();
  const createDialog = page.getByRole("dialog", { name: "Tạo chương trình mẫu" });
  await createDialog.getByLabel("Mã chương trình").fill(RUN_CODE.toLowerCase());
  await createDialog.getByLabel("Tên chương trình").fill(TEMPLATE_NAME);
  await createDialog.getByLabel("Môn học").fill("Toán");
  await createDialog.getByLabel("Trình độ").fill("Lớp 6");
  await createDialog.getByRole("button", { name: "Tạo" }).click();

  // Creating lands on the detail page with an empty v1 draft.
  await expect(page).toHaveURL(/\/library\/templates\/[0-9a-f-]+$/);
  await expect(page.getByRole("heading", { name: TEMPLATE_NAME })).toBeVisible();
  await expect(page.getByText(`Mã: ${RUN_CODE}`)).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Phiên bản" })).toHaveText(/v1 · Bản nháp/);
  await expect(page.getByText("Phiên bản này chưa có buổi học nào.")).toBeVisible();

  for (const [title, duration] of [
    ["Số tự nhiên", "90"],
    ["Phân số", "60"],
  ] as const) {
    await page.getByRole("button", { name: "Thêm buổi học" }).click();
    const dialog = page.getByRole("dialog", { name: "Thêm buổi học" });
    await dialog.getByLabel("Tên buổi").fill(title);
    await dialog.getByLabel("Thời lượng (phút)").fill(duration);
    await dialog.getByLabel("Bài tập về nhà").fill(`Bài tập ${title}`);
    await dialog.getByRole("button", { name: "Thêm" }).click();
    await expect(page.getByRole("row").filter({ hasText: title })).toBeVisible();
  }
  const rows = page.getByRole("row");
  await expect(rows.nth(1)).toContainText("Số tự nhiên");
  await expect(rows.nth(2)).toContainText("Phân số");

  // Move the second lesson up: the positions renumber to match.
  await rows.nth(2).getByRole("button", { name: "Chuyển lên" }).click();
  await expect(rows.nth(1)).toContainText("Phân số");
  await expect(rows.nth(2)).toContainText("Số tự nhiên");

  // Edit one lesson on its own page.
  await rows.nth(1).getByRole("link", { name: "Phân số" }).click();
  await expect(page.getByRole("heading", { name: "Buổi 1 · Phân số" })).toBeVisible();
  await page.getByLabel("Mục tiêu").fill("So sánh hai phân số");
  await page.getByRole("button", { name: "Lưu", exact: true }).click();
  await expect(page.getByText("Đã lưu buổi học")).toBeVisible();
  await page.getByRole("link", { name: TEMPLATE_NAME }).click();

  // Publish: the draft becomes v1 published and the lessons lock.
  await page.getByRole("button", { name: "Phát hành" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Phát hành" }).click();
  await expect(page.getByText("Đã phát hành v1")).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Phiên bản" })).toHaveText(/v1 · Đã phát hành/);
  await expect(page.getByText(/Phiên bản này đã phát hành/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Thêm buổi học" })).toBeHidden();
  await expect(page.getByRole("button", { name: "Chuyển lên" })).toHaveCount(0);

  // The locked lesson opens read-only with the saved objectives.
  await page.getByRole("link", { name: "Phân số" }).click();
  await expect(page.getByText(/Phiên bản v1 đã phát hành/)).toBeVisible();
  await expect(page.getByRole("group", { name: "Mục tiêu" })).toContainText("So sánh hai phân số");
  await expect(page.getByRole("button", { name: "Lưu", exact: true })).toBeHidden();
  await page.getByRole("link", { name: TEMPLATE_NAME }).click();

  // A new draft copies the published lessons and reopens authoring.
  await page.getByRole("button", { name: "Tạo bản nháp mới" }).click();
  const draftDialog = page.getByRole("dialog", { name: "Tạo bản nháp mới" });
  await draftDialog.getByLabel("Ghi chú thay đổi").fill("Thêm buổi ôn tập");
  await draftDialog.getByRole("button", { name: "Tạo bản nháp" }).click();
  await expect(page.getByText("Đã tạo bản nháp v2")).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Phiên bản" })).toHaveText(/v2 · Bản nháp/);
  await expect(page.getByRole("row").filter({ hasText: "Phân số" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Thêm buổi học" })).toBeVisible();

  // The catalog row carries both versions.
  await page.goto("/library");
  const catalogRow = page.getByRole("row").filter({ hasText: RUN_CODE });
  await expect(catalogRow).toContainText("v1 · Đã phát hành");
  await expect(catalogRow).toContainText("v2 · Bản nháp");
});
