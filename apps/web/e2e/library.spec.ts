import { expect, test, type Page } from "@playwright/test";

import { loginAsOwner } from "./helpers/auth.js";

// Dev-only seed: Cô Lan owns the center, so she holds every library key. The
// template code carries a per-run suffix, which keeps the journey clear of the
// duplicate-code 409 on a reused database, and the afterEach deletes whatever
// the run created so the catalog stays as the seed left it.
const RUN_CODE = `E2E-${Date.now().toString(36).toUpperCase()}`.slice(0, 20);
const TEMPLATE_NAME = `Chương trình e2e ${RUN_CODE}`;
const MATERIAL_TITLE = `Slide ${RUN_CODE}`;
const EXERCISE_TITLE = `Bài tập ${RUN_CODE}`;

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

/**
 * Deletes the run's catalog item on one tab, if present. A template's soft
 * delete keeps its lessons' links, so the journey detaches first; this only
 * has to be tolerant of an item that is already gone.
 */
async function deleteRunItem(page: Page, tab: "materials" | "exercises", title: string) {
  const noun = tab === "materials" ? "học liệu" : "bài tập";
  await page.goto(`/library?tab=${tab}`);
  await page.getByRole("searchbox", { name: `Tìm ${noun}` }).fill(RUN_CODE);
  const row = page.getByRole("row").filter({ hasText: title });
  const noMatch = page.getByText(`Không có ${noun} nào khớp từ khoá.`);
  await expect(row.or(noMatch)).toBeVisible();
  if ((await row.count()) === 0) return;

  await row.getByRole("button", { name: `Xoá ${title}` }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: `Xoá ${noun}` })
    .click();
  // A run that failed before detaching leaves the item linked; the refusal
  // is reported by the test itself, so the cleanup only has to not cascade.
  await expect(
    page.getByText(`Đã xoá ${noun}`).or(page.getByText(/đang được gắn vào buổi học/)),
  ).toBeVisible();
}

test.afterEach(async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  // The template goes first: links kept by a deleted template no longer
  // hold its items. Each step runs on its own so one failure does not
  // leave the others behind.
  const errors: unknown[] = [];
  try {
    await loginAsOwner(page);
    for (const step of [
      () => deleteRunTemplate(page),
      () => deleteRunItem(page, "materials", MATERIAL_TITLE),
      () => deleteRunItem(page, "exercises", EXERCISE_TITLE),
    ]) {
      try {
        await step();
      } catch (error) {
        errors.push(error);
      }
    }
  } finally {
    await context.close();
  }
  if (errors.length > 0) throw errors[0];
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

test("owner catalogs a material and an exercise, attaches them to a draft lesson and sets up grading", async ({
  page,
}) => {
  await loginAsOwner(page);

  // Catalog: one material with a link and tags, one exercise with a difficulty.
  await page.goto("/library?tab=materials");
  await expect(page.getByRole("tab", { name: "Học liệu" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.getByRole("button", { name: "Thêm học liệu" }).click();
  const materialDialog = page.getByRole("dialog", { name: "Thêm học liệu" });
  await materialDialog.getByLabel("Tên học liệu").fill(MATERIAL_TITLE);
  await materialDialog.getByRole("combobox", { name: "Loại" }).click();
  await page.getByRole("option", { name: "Tài liệu" }).click();
  await materialDialog.getByLabel("Đường dẫn").fill("https://example.com/slide");
  await materialDialog.getByLabel("Thẻ").fill("e2e, chương 1");
  await materialDialog.getByRole("button", { name: "Thêm" }).click();
  await expect(page.getByText("Đã thêm học liệu")).toBeVisible();
  const materialRow = page.getByRole("row").filter({ hasText: MATERIAL_TITLE });
  await expect(materialRow).toContainText("Tài liệu");
  await expect(materialRow).toContainText("e2e, chương 1");

  await page.getByRole("tab", { name: "Bài tập" }).click();
  await page.getByRole("button", { name: "Thêm bài tập" }).click();
  const exerciseDialog = page.getByRole("dialog", { name: "Thêm bài tập" });
  await exerciseDialog.getByLabel("Tên bài tập").fill(EXERCISE_TITLE);
  await exerciseDialog.getByRole("combobox", { name: "Độ khó" }).click();
  await page.getByRole("option", { name: "Mức 3" }).click();
  await exerciseDialog.getByRole("button", { name: "Thêm" }).click();
  await expect(page.getByText("Đã thêm bài tập")).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: EXERCISE_TITLE })).toContainText("Mức 3");

  // A fresh draft with one lesson to attach to.
  await page.getByRole("tab", { name: "Chương trình mẫu" }).click();
  await page.getByRole("button", { name: "Tạo chương trình mẫu" }).click();
  const createDialog = page.getByRole("dialog", { name: "Tạo chương trình mẫu" });
  await createDialog.getByLabel("Mã chương trình").fill(RUN_CODE.toLowerCase());
  await createDialog.getByLabel("Tên chương trình").fill(TEMPLATE_NAME);
  await createDialog.getByRole("button", { name: "Tạo" }).click();
  await expect(page.getByRole("heading", { name: TEMPLATE_NAME })).toBeVisible();
  const templateUrl = page.url();

  await page.getByRole("button", { name: "Thêm buổi học" }).click();
  const lessonDialog = page.getByRole("dialog", { name: "Thêm buổi học" });
  await lessonDialog.getByLabel("Tên buổi").fill("Số tự nhiên");
  await lessonDialog.getByRole("button", { name: "Thêm" }).click();
  await page.getByRole("link", { name: "Số tự nhiên" }).click();
  await expect(page.getByRole("heading", { name: "Buổi 1 · Số tự nhiên" })).toBeVisible();

  // Attach: tick, share with students, save; the exercise block saves on its own.
  const materials = page.getByRole("region", { name: "Học liệu" });
  await materials.getByRole("searchbox", { name: "Tìm học liệu" }).fill(RUN_CODE);
  await materials.getByRole("checkbox", { name: MATERIAL_TITLE, exact: true }).check();
  await materials.getByRole("checkbox", { name: `Chia sẻ ${MATERIAL_TITLE} với học viên` }).check();
  await materials.getByRole("button", { name: "Lưu học liệu" }).click();
  await expect(page.getByText("Đã lưu học liệu của buổi")).toBeVisible();

  const exercises = page.getByRole("region", { name: "Bài tập" });
  await exercises.getByRole("searchbox", { name: "Tìm bài tập" }).fill(RUN_CODE);
  await exercises.getByRole("checkbox", { name: EXERCISE_TITLE, exact: true }).check();
  await exercises.getByRole("button", { name: "Lưu bài tập" }).click();
  await expect(page.getByText("Đã lưu bài tập của buổi")).toBeVisible();

  // The attachments survive a reload.
  await page.reload();
  await expect(page.getByRole("heading", { name: "Buổi 1 · Số tự nhiên" })).toBeVisible();
  await expect(
    materials.getByRole("checkbox", { name: MATERIAL_TITLE, exact: true }),
  ).toBeChecked();
  await expect(
    materials.getByRole("checkbox", { name: `Chia sẻ ${MATERIAL_TITLE} với học viên` }),
  ).toBeChecked();
  await expect(
    exercises.getByRole("checkbox", { name: EXERCISE_TITLE, exact: true }),
  ).toBeChecked();

  // An attached material cannot be deleted from the catalog.
  await page.goto("/library?tab=materials");
  await page.getByRole("searchbox", { name: "Tìm học liệu" }).fill(RUN_CODE);
  await page.getByRole("button", { name: `Xoá ${MATERIAL_TITLE}` }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Xoá học liệu" }).click();
  await expect(page.getByText(/đang được gắn vào buổi học/)).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: MATERIAL_TITLE })).toBeVisible();

  // Grading: one select log field and one score component on the draft.
  await page.goto(templateUrl);
  await page.getByRole("tab", { name: "Nhật ký & Điểm" }).click();
  const fields = page.getByRole("region", { name: "Trường nhật ký" });
  await fields.getByRole("button", { name: "Thêm trường" }).click();
  await fields.getByRole("textbox", { name: "Nhãn trường 1" }).fill("Mức độ tập trung");
  await fields.getByRole("combobox", { name: "Loại trường 1" }).click();
  await page.getByRole("option", { name: "Chọn một" }).click();
  await fields.getByRole("textbox", { name: "Tuỳ chọn trường 1" }).fill("Tốt, Khá");
  await fields.getByRole("checkbox", { name: "Bắt buộc trường 1" }).check();
  await fields.getByRole("button", { name: "Lưu trường nhật ký" }).click();
  await expect(page.getByText("Đã lưu trường nhật ký")).toBeVisible();

  const scores = page.getByRole("region", { name: "Cơ cấu điểm" });
  await scores.getByRole("button", { name: "Thêm thành phần" }).click();
  await scores.getByRole("textbox", { name: "Mã thành phần 1" }).fill("kt");
  await scores.getByRole("textbox", { name: "Tên thành phần 1" }).fill("Kiểm tra");
  await scores.getByRole("spinbutton", { name: "Trọng số 1" }).fill("1");
  await scores.getByRole("button", { name: "Lưu cơ cấu điểm" }).click();
  await expect(page.getByText("Đã lưu cơ cấu điểm")).toBeVisible();

  await page.reload();
  await page.getByRole("tab", { name: "Nhật ký & Điểm" }).click();
  await expect(fields.getByRole("textbox", { name: "Nhãn trường 1" })).toHaveValue(
    "Mức độ tập trung",
  );
  await expect(fields.getByRole("textbox", { name: "Tuỳ chọn trường 1" })).toHaveValue("Tốt, Khá");
  await expect(scores.getByRole("textbox", { name: "Mã thành phần 1" })).toHaveValue("kt");
  await expect(scores.getByRole("spinbutton", { name: "Trọng số 1" })).toHaveValue("1");

  // Detach both blocks: the picker must let a linked item go on a draft.
  await page.getByRole("tab", { name: "Buổi học" }).click();
  await page.getByRole("link", { name: "Số tự nhiên" }).click();
  await materials.getByRole("checkbox", { name: MATERIAL_TITLE, exact: true }).uncheck();
  await materials.getByRole("button", { name: "Lưu học liệu" }).click();
  await expect(page.getByText("Đã lưu học liệu của buổi")).toBeVisible();
  await exercises.getByRole("checkbox", { name: EXERCISE_TITLE, exact: true }).uncheck();
  await exercises.getByRole("button", { name: "Lưu bài tập" }).click();
  await expect(page.getByText("Đã lưu bài tập của buổi")).toBeVisible();
});
