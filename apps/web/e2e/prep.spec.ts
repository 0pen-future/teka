import { expect, test, type Page } from "@playwright/test";

import { loginAsOwner } from "./helpers/auth.js";

// Dev-only seed: Cô Lan owns the center, so she holds library.edit and
// prep.assign. The template code carries a per-run suffix, which keeps the
// journey clear of the duplicate-code 409 on a reused database, and the
// afterEach deletes whatever the run created so the catalog stays as the
// seed left it.
const RUN_CODE = `E2E-${Date.now().toString(36).toUpperCase()}`.slice(0, 20);
const TEMPLATE_NAME = `Chuẩn bị e2e ${RUN_CODE}`;

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
  await loginAsOwner(page);
  await deleteRunTemplate(page);
  await context.close();
});

test("owner creates a template in the wizard, then prepares its lessons", async ({ page }) => {
  await loginAsOwner(page);

  // Wizard: template fields, lesson count, confirm.
  await page.goto("/library/templates/new");
  await page.getByLabel("Mã chương trình").fill(RUN_CODE);
  await page.getByLabel("Tên chương trình").fill(TEMPLATE_NAME);
  await page.getByLabel("Môn học").fill("Toán");
  await page.getByRole("button", { name: "Tiếp tục" }).click();
  await expect(page.getByRole("heading", { name: /Bước 2/ })).toBeVisible();
  await page.getByLabel("Số buổi").fill("3");
  await page.getByRole("button", { name: "Tiếp tục" }).click();
  await expect(page.getByRole("heading", { name: /Bước 3/ })).toBeVisible();
  await expect(page.getByText("Buổi 3")).toBeVisible();
  await page.getByRole("button", { name: "Tạo chương trình" }).click();

  // Lands on the draft's board with the three pre-created lessons.
  await expect(page).toHaveURL(/\/prep\/[0-9a-f-]+\/board$/);
  await expect(page.getByRole("heading", { name: new RegExp(TEMPLATE_NAME) })).toBeVisible();
  const todo = page.getByRole("listbox", { name: "Cần làm" });
  await expect(todo.getByRole("option")).toHaveCount(3);

  // Move Buổi 1 through the card menu.
  await todo
    .getByRole("option", { name: /Buổi 1/ })
    .getByRole("button", { name: "Thao tác" })
    .click();
  await page.getByRole("menuitem", { name: "Đang làm" }).click();
  const doing = page.getByRole("listbox", { name: "Đang làm" });
  await expect(doing.getByRole("option", { name: /Buổi 1/ })).toBeVisible();
  await expect(todo.getByRole("option")).toHaveCount(2);

  // Assign Thầy Minh with a due date; each change is one full-replace PATCH.
  await page.getByRole("link", { name: "Phân công" }).click();
  await expect(page).toHaveURL(/\/assign$/);
  const row = page.getByRole("row").filter({ hasText: "Buổi 1" });
  const assigned = page.waitForResponse(
    (response) => response.url().includes("/assignment") && response.request().method() === "PATCH",
  );
  await row.getByRole("combobox", { name: /Người phụ trách/ }).click();
  await page.getByRole("option", { name: /Thầy Minh/ }).click();
  await assigned;
  await expect(page.getByText("Đã lưu phân công").first()).toBeVisible();
  const dated = page.waitForResponse(
    (response) => response.url().includes("/assignment") && response.request().method() === "PATCH",
  );
  await row.getByLabel("Hạn hoàn thành").fill("2026-10-01");
  await dated;

  // The board card now carries the assignee and the due date.
  await page.getByRole("link", { name: "Bảng chuẩn bị" }).click();
  const card = page
    .getByRole("listbox", { name: "Đang làm" })
    .getByRole("option", { name: /Buổi 1/ });
  await expect(card).toContainText("Thầy Minh");
  await expect(card).toContainText("01/10");

  // Open the lesson, work its checklist and mark it done.
  await card.getByRole("button", { name: "Thao tác" }).click();
  await page.getByRole("menuitem", { name: "Mở chi tiết" }).click();
  await expect(page).toHaveURL(/\/library\/templates\/[0-9a-f-]+\/lessons\/[0-9a-f-]+$/);
  const prep = page.getByRole("region", { name: "Chuẩn bị tài liệu" });
  await expect(prep).toContainText("Thầy Minh");
  const newItem = prep.getByLabel("Thêm việc cần làm");
  await newItem.fill("Soạn slide");
  await newItem.press("Enter");
  await newItem.fill("In phiếu bài tập");
  await newItem.press("Enter");
  await prep.getByRole("checkbox", { name: "Soạn slide" }).check();
  await prep.getByRole("radio", { name: "Hoàn thành" }).click();
  await prep.getByRole("button", { name: "Lưu chuẩn bị" }).click();
  await expect(page.getByText("Đã lưu trạng thái chuẩn bị")).toBeVisible();

  // The list page reflects the finished lesson and its assignee.
  await page.goto("/prep");
  const listRow = page.getByRole("row").filter({ hasText: TEMPLATE_NAME });
  await expect(listRow).toContainText("1/3 buổi");
  await expect(listRow).toContainText("Thầy Minh");
  await expect(listRow.getByRole("link", { name: "Bảng chuẩn bị" })).toBeVisible();
});
