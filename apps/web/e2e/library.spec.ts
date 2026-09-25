import { expect, test, type Page } from "@playwright/test";

import { loginAsOwner } from "./helpers/auth.js";

// Dev-only seed: Cô Lan owns the center, so she holds every library key. The
// template/material/exercise names carry a per-run suffix, which keeps the
// journey clear of the duplicate-name/code 409 on a reused database, and the
// afterEach deletes whatever the run created so the catalog stays as the
// seed left it.
const RUN_CODE = `E2E-${Date.now().toString(36).toUpperCase()}`.slice(0, 20);
const TEMPLATE_NAME = `Chương trình e2e ${RUN_CODE}`;
const MATERIAL_TITLE = `Học liệu e2e ${RUN_CODE}`;
const EXERCISE_TITLE = `Bài tập e2e ${RUN_CODE}`;
const EXERCISE_GROUP_NAME = `Nhóm nâng cao ${RUN_CODE}`;

async function pickOption(page: Page, combobox: string, option: RegExp | string) {
  await page.getByRole("combobox", { name: combobox }).click();
  await page.getByRole("listbox").getByRole("option", { name: option }).click();
}

/** Deletes the template this run created, if it is still in the catalog. */
async function deleteRunTemplate(page: Page) {
  await page.goto("/library");
  await page.getByRole("searchbox", { name: "Tìm chương trình mẫu" }).fill(RUN_CODE);
  const card = page.getByRole("article", { name: TEMPLATE_NAME });
  const noMatch = page.getByText("Không có chương trình nào khớp từ khoá.");
  await expect(card.or(noMatch)).toBeVisible();
  if ((await card.count()) === 0) return;

  await card.getByRole("link", { name: TEMPLATE_NAME }).click();
  await page.getByRole("button", { name: "Xoá chương trình" }).click();
  const dialog = page.getByRole("dialog", { name: `Xoá chương trình "${TEMPLATE_NAME}"?` });
  await dialog.getByRole("button", { name: "Xoá chương trình" }).click();
  await expect(page.getByText(`Đã xoá chương trình ${RUN_CODE}`)).toBeVisible();
}

/**
 * Deletes the run's catalog item on one bank tab, if present. A template's
 * delete is a soft delete that never releases a lesson-material or
 * lesson-exercise link, so an item this run attached to a lesson stays
 * permanently linked and its delete button stays disabled — this only has
 * to skip that case cleanly instead of failing the run.
 */
async function deleteRunItem(page: Page, tab: "materials" | "exercises", title: string) {
  const noun = tab === "materials" ? "học liệu" : "bài tập";
  const searchLabel = tab === "materials" ? "Tìm học liệu" : "Tìm bài tập";
  await page.goto(`/library/${tab}`);
  await page.getByRole("searchbox", { name: searchLabel }).fill(RUN_CODE);
  const deleteButton = page.getByRole("button", { name: `Xoá ${title}` });
  const noMatch = page.getByText(`Không có ${noun} nào khớp từ khoá.`);
  await expect(deleteButton.or(noMatch)).toBeVisible();
  if ((await deleteButton.count()) === 0) return;
  if (await deleteButton.isDisabled()) return;

  await deleteButton.click();
  const dialog = page.getByRole("dialog", { name: `Xoá ${noun} "${title}"?` });
  await dialog.getByRole("button", { name: `Xoá ${noun}` }).click();
  await expect(page.getByText(`Đã xoá ${noun}`)).toBeVisible();
}

test.afterEach(async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  // The template goes first, then the exercise bank, then the material bank.
  // Each step runs on its own so one failure does not leave the others
  // behind.
  const errors: unknown[] = [];
  try {
    await loginAsOwner(page);
    for (const step of [
      () => deleteRunTemplate(page),
      () => deleteRunItem(page, "exercises", EXERCISE_TITLE),
      () => deleteRunItem(page, "materials", MATERIAL_TITLE),
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

test("owner builds a template through the hub, its lessons, groups, grading and version lifecycle", async ({
  page,
}) => {
  // This journey walks the whole v5 hub end to end — hub tabs, template
  // lessons, tree view, exercise groups, score sets, log fields, and the
  // publish/draft lifecycle — so it needs more than the suite's default
  // per-test budget.
  test.setTimeout(120_000);
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await loginAsOwner(page);

  // The hub has three tabs: template catalog, content bank, exercise bank.
  await page.goto("/library");
  await expect(page.getByRole("heading", { name: "Kho học liệu" })).toBeVisible();
  const hubTabs = page.getByRole("tablist", { name: "Các mục của kho học liệu" });
  await expect(hubTabs.getByRole("tab", { name: "Chương trình mẫu" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(hubTabs.getByRole("tab", { name: "Ngân hàng nội dung" })).toBeVisible();
  await expect(hubTabs.getByRole("tab", { name: "Ngân hàng bài tập" })).toBeVisible();

  // Catalog one material, active by default.
  await hubTabs.getByRole("tab", { name: "Ngân hàng nội dung" }).click();
  await expect(page).toHaveURL(/\/library\/materials$/);
  await page.getByRole("button", { name: "Thêm học liệu" }).click();
  const materialDialog = page.getByRole("dialog", { name: "Thêm học liệu" });
  await materialDialog.getByLabel("Tên học liệu").fill(MATERIAL_TITLE);
  await materialDialog.getByLabel("Đường dẫn").fill("https://example.com/slide");
  await materialDialog.getByRole("button", { name: "Thêm" }).click();
  await expect(page.getByText("Đã thêm học liệu")).toBeVisible();

  // Catalog one exercise without a code: the server auto-generates a BT- code,
  // and its copy button both shows and copies that code.
  await hubTabs.getByRole("tab", { name: "Ngân hàng bài tập" }).click();
  await expect(page).toHaveURL(/\/library\/exercises$/);
  await page.getByRole("button", { name: "Thêm bài tập" }).click();
  const exerciseDialog = page.getByRole("dialog", { name: "Thêm bài tập" });
  await exerciseDialog.getByLabel("Tên bài tập").fill(EXERCISE_TITLE);
  await exerciseDialog.getByRole("button", { name: "Thêm" }).click();
  await expect(page.getByText("Đã thêm bài tập")).toBeVisible();
  await page.getByRole("searchbox", { name: "Tìm bài tập" }).fill(RUN_CODE);
  const exerciseRow = page.getByRole("row").filter({ hasText: EXERCISE_TITLE });
  const copyButton = exerciseRow.getByRole("button", { name: /^Sao chép mã / });
  await expect(copyButton).toBeVisible();
  const generatedCode = (await copyButton.getAttribute("aria-label"))!.replace("Sao chép mã ", "");
  expect(generatedCode).toMatch(/^BT-/);
  await copyButton.click();
  await expect(page.getByText("Đã sao chép mã")).toBeVisible();
  const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
  expect(clipboardText).toBe(generatedCode);

  // Create the template that the rest of the journey builds on.
  await hubTabs.getByRole("tab", { name: "Chương trình mẫu" }).click();
  await expect(page).toHaveURL(/\/library$/);
  await page.getByRole("button", { name: "+ Chương trình mẫu" }).click();
  const templateDialog = page.getByRole("dialog", { name: "Tạo chương trình mẫu" });
  await templateDialog.getByLabel("Mã chương trình").fill(RUN_CODE.toLowerCase());
  await templateDialog.getByLabel("Tên chương trình").fill(TEMPLATE_NAME);
  await templateDialog.getByLabel("Môn học").fill("Toán");
  await templateDialog.getByLabel("Trình độ").fill("Lớp 6");
  await templateDialog.getByRole("button", { name: "Tạo" }).click();
  await expect(page).toHaveURL(/\/library\/templates\/[0-9a-f-]+$/);
  await expect(page.getByRole("heading", { name: TEMPLATE_NAME })).toBeVisible();
  await expect(
    page.getByRole("radiogroup", { name: "Phiên bản" }).getByRole("radio", {
      name: "v1 · Bản nháp",
    }),
  ).toHaveAttribute("aria-checked", "true");

  // Three lessons across two units, to exercise the tree grouping later.
  async function addLesson(title: string, unit: string, durationMin: number) {
    await page.getByRole("button", { name: "+ Buổi học" }).click();
    await page.getByRole("menuitem", { name: "Thêm một buổi" }).click();
    const dialog = page.getByRole("dialog", { name: "Thêm buổi học" });
    await dialog.getByLabel("Tên buổi").fill(title);
    await dialog.getByLabel("Thời lượng (phút)").fill(String(durationMin));
    await dialog.getByLabel("Đơn vị").fill(unit);
    await dialog.getByRole("button", { name: "Thêm" }).click();
    await expect(page.getByRole("link", { name: title })).toBeVisible();
  }

  await addLesson("Số tự nhiên", "Tổ Toán", 90);
  await addLesson("Phân số", "Tổ Toán", 60);
  await addLesson("Ngữ pháp", "Tổ Văn", 45);

  // Duplicate the last lesson: the copy keeps the same unit, right after it.
  const nguPhapRow = page.getByRole("row").filter({ hasText: "Ngữ pháp" }).first();
  await nguPhapRow.getByRole("button", { name: "Nhân bản" }).click();
  await expect(page.getByText("Đã nhân bản buổi học")).toBeVisible();
  await expect(page.getByRole("link", { name: "Ngữ pháp (bản sao)" })).toBeVisible();

  // Tree view groups the four lessons by unit.
  await page.getByRole("radio", { name: "Cây" }).click();
  const toanGroup = page.getByRole("button", { name: /Tổ Toán/ });
  await expect(toanGroup).toContainText("2 buổi · 150 phút");
  const vanGroup = page.getByRole("button", { name: /Tổ Văn/ });
  await expect(vanGroup).toContainText("2 buổi · 90 phút");

  // The hub card reflects the four lessons in this one draft version.
  await page.goto("/library");
  const templateCard = page.getByRole("article", { name: TEMPLATE_NAME });
  await expect(templateCard).toContainText("4 buổi mẫu · 1 phiên bản · 0 lớp đang gắn");
  await templateCard.getByRole("link", { name: TEMPLATE_NAME }).click();

  // An exercise group, then attach the catalog exercise to it from a lesson.
  await page.getByRole("tab", { name: "Nhóm bài tập" }).click();
  await page.getByRole("textbox", { name: "Tên nhóm mới…" }).fill(EXERCISE_GROUP_NAME);
  await page.getByRole("textbox", { name: "Tên nhóm mới…" }).press("Enter");
  await expect(page.getByText("Đã thêm nhóm bài tập")).toBeVisible();
  const groupRow = page.getByRole("listitem").filter({ hasText: EXERCISE_GROUP_NAME });
  await expect(groupRow).toContainText("Dùng trong 0 bài");

  await page.getByRole("tab", { name: "Buổi học" }).click();
  await page.getByRole("link", { name: "Số tự nhiên", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Buổi 1 · Số tự nhiên" })).toBeVisible();

  // The content picker offers the still-active material; close without adding.
  const contents = page.getByRole("region", { name: "Nội dung buổi học" });
  await contents.getByRole("button", { name: "Thêm nội dung" }).click();
  await page.getByRole("menuitem", { name: "Chọn từ ngân hàng nội dung" }).click();
  const materialPicker = page.getByRole("dialog", { name: "Chọn từ ngân hàng nội dung" });
  await materialPicker.getByRole("searchbox", { name: "Tìm nội dung" }).fill(RUN_CODE);
  await expect(
    materialPicker.getByRole("checkbox", { name: MATERIAL_TITLE, exact: true }),
  ).toBeVisible();
  await materialPicker.getByRole("button", { name: "Hủy" }).click();

  // Attach the exercise from the bank picker, then assign it to the group.
  await page.getByRole("tab", { name: "Bài tập" }).click();
  await page.getByRole("button", { name: "Bài tập có sẵn" }).click();
  const exercisePicker = page.getByRole("dialog", { name: "Chọn từ ngân hàng bài tập" });
  await exercisePicker.getByRole("searchbox", { name: "Tìm bài tập" }).fill(RUN_CODE);
  await exercisePicker.getByRole("checkbox", { name: EXERCISE_TITLE, exact: true }).check();
  await exercisePicker.getByRole("button", { name: "Thêm 1 bài tập" }).click();
  await expect(page.getByText("Đã thêm bài tập vào buổi học")).toBeVisible();

  await pickOption(page, `Nhóm bài tập cho ${EXERCISE_TITLE}`, EXERCISE_GROUP_NAME);
  await expect(page.getByText("Đã cập nhật nhóm bài tập")).toBeVisible();

  await page.getByRole("link", { name: TEMPLATE_NAME }).click();
  await page.getByRole("tab", { name: "Nhóm bài tập" }).click();
  await expect(groupRow).toContainText("Dùng trong 1 bài");

  // Archiving the material removes it from the content picker's active list.
  await page.goto("/library/materials");
  await page.getByRole("searchbox", { name: "Tìm học liệu" }).fill(RUN_CODE);
  await page.getByRole("button", { name: `Ngừng ${MATERIAL_TITLE}` }).click();
  await expect(page.getByText("Đã ngừng học liệu")).toBeVisible();

  await page.goto("/library");
  await page.getByRole("searchbox", { name: "Tìm chương trình mẫu" }).fill(RUN_CODE);
  await page
    .getByRole("article", { name: TEMPLATE_NAME })
    .getByRole("link", {
      name: TEMPLATE_NAME,
    })
    .click();
  await page.getByRole("tab", { name: "Buổi học" }).click();
  await page.getByRole("link", { name: "Số tự nhiên", exact: true }).click();
  await contents.getByRole("button", { name: "Thêm nội dung" }).click();
  await page.getByRole("menuitem", { name: "Chọn từ ngân hàng nội dung" }).click();
  await materialPicker.getByRole("searchbox", { name: "Tìm nội dung" }).fill(RUN_CODE);
  await expect(materialPicker.getByText("Không có nội dung nào khớp từ khoá.")).toBeVisible();
  await materialPicker.getByRole("button", { name: "Hủy" }).click();

  // Two score-set groups; only the group-scoped "Xoá bộ"/"+ Thêm điểm thành
  // phần" buttons repeat unlabeled per group, so they need index scoping.
  await page.getByRole("link", { name: TEMPLATE_NAME }).click();
  await page.getByRole("tab", { name: "Bộ điểm" }).click();
  await page.getByRole("button", { name: "+ Thêm bộ điểm" }).click();
  await page.getByRole("textbox", { name: "Tiêu đề bộ điểm 1" }).fill("Giữa kỳ");
  await page.getByRole("button", { name: "+ Thêm điểm thành phần" }).nth(0).click();
  await page.getByRole("textbox", { name: "Mã thành phần 1.1" }).fill("kt1");
  await page.getByRole("textbox", { name: "Tên thành phần 1.1" }).fill("Kiểm tra 1");

  await page.getByRole("button", { name: "+ Thêm bộ điểm" }).click();
  await page.getByRole("textbox", { name: "Tiêu đề bộ điểm 2" }).fill("Cuối kỳ");
  await page.getByRole("button", { name: "+ Thêm điểm thành phần" }).nth(1).click();
  await page.getByRole("textbox", { name: "Mã thành phần 2.1" }).fill("kt2");
  await page.getByRole("textbox", { name: "Tên thành phần 2.1" }).fill("Kiểm tra cuối kỳ");

  await page.getByRole("button", { name: "Lưu cơ cấu điểm" }).click();
  await expect(page.getByText("Đã lưu cơ cấu điểm")).toBeVisible();
  await page.reload();
  await page.getByRole("tab", { name: "Bộ điểm" }).click();
  await expect(page.getByRole("textbox", { name: "Tiêu đề bộ điểm 1" })).toHaveValue("Giữa kỳ");
  await expect(page.getByRole("textbox", { name: "Tiêu đề bộ điểm 2" })).toHaveValue("Cuối kỳ");

  // One student-picker log field.
  await page.getByRole("tab", { name: "Nhật ký" }).click();
  await page.getByRole("button", { name: "Thêm trường" }).click();
  await page.getByRole("textbox", { name: "Nhãn trường 1" }).fill("Học sinh vắng");
  await pickOption(page, "Loại trường 1", "Chọn học sinh");
  await page.getByRole("button", { name: "Lưu trường nhật ký" }).click();
  await expect(page.getByText("Đã lưu trường nhật ký")).toBeVisible();

  // Publish v1: lessons lock, and the banner/version chip reflect it.
  await page.getByRole("button", { name: "Kích hoạt" }).click();
  const publishDialog = page.getByRole("dialog", { name: "Kích hoạt v1?" });
  await publishDialog.getByRole("button", { name: "Kích hoạt" }).click();
  await expect(page.getByText("Đã kích hoạt v1")).toBeVisible();
  await expect(
    page.getByText(
      "Phiên bản đã phát hành (0 lớp đang gắn) — chỉ xem. Muốn sửa, tạo bản nháp mới.",
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("radiogroup", { name: "Phiên bản" }).getByRole("radio", {
      name: "v1 · Đã phát hành",
    }),
  ).toHaveAttribute("aria-checked", "true");

  await page.getByRole("tab", { name: "Buổi học" }).click();
  await page.getByRole("link", { name: "Số tự nhiên", exact: true }).click();
  await expect(
    page.getByText(
      "Phiên bản v1 đã phát hành (0 lớp đang gắn) — chỉ xem. Muốn sửa, tạo bản nháp mới ở màn chương trình mẫu.",
    ),
  ).toBeVisible();

  // A new draft copies the published version's exercise group forward.
  await page.getByRole("link", { name: TEMPLATE_NAME }).click();
  await page.getByRole("button", { name: "Tạo bản nháp" }).click();
  const draftDialog = page.getByRole("dialog", { name: "Tạo bản nháp mới" });
  await draftDialog.getByLabel("Ghi chú thay đổi").fill("Thêm buổi ôn tập");
  await draftDialog.getByRole("button", { name: "Tạo bản nháp" }).click();
  await expect(page.getByText("Đã tạo bản nháp v2")).toBeVisible();
  await expect(
    page.getByRole("radiogroup", { name: "Phiên bản" }).getByRole("radio", {
      name: "v2 · Bản nháp",
    }),
  ).toHaveAttribute("aria-checked", "true");

  await page.getByRole("tab", { name: "Nhóm bài tập" }).click();
  const copiedGroupRow = page.getByRole("listitem").filter({ hasText: EXERCISE_GROUP_NAME });
  await expect(copiedGroupRow).toContainText("Dùng trong 1 bài");
});
