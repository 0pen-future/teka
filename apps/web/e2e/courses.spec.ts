import { expect, test, type Page } from "@playwright/test";

import { apiLoginAsOwner, loginAsOwner } from "./helpers/auth.js";

// Dev-only seed: Cô Lan owns the center, so she holds the course keys and
// the class keys. The course code carries a per-run suffix so a reused
// database never trips the duplicate-code 409, and the afterEach removes
// the class and the course the run created through the API, since a course
// refuses to be deleted while a class is still attached.
const RUN_CODE = `E2E-${Date.now().toString(36).toUpperCase()}`.slice(0, 20);
const COURSE_NAME = `Khóa e2e ${RUN_CODE}`;
const COURSE_NAME_EDITED = `${COURSE_NAME} (sửa)`;
const CLASS_NAME = `Lớp e2e ${RUN_CODE}`;

/** Ids the journey captured, so cleanup does not have to search for them. */
const created: { courseId?: string; classId?: string } = {};

async function pickOption(page: Page, combobox: string, option: RegExp | string) {
  await page.getByRole("combobox", { name: combobox }).click();
  await page.getByRole("listbox").getByRole("option", { name: option }).click();
}

test.afterEach(async ({ request }) => {
  const token = await apiLoginAsOwner(request);
  const headers = { Authorization: `Bearer ${token}` };
  // Each step reports its own failure and lets the next one run, so a class
  // that would not delete still leaves the course attempt (and its 409) in
  // the log instead of silently leaking both rows.
  const failures: string[] = [];
  if (created.classId) {
    const response = await request.delete(`/api/v1/classes/${created.classId}`, { headers });
    if (![200, 204, 404].includes(response.status())) {
      failures.push(`class ${created.classId}: ${response.status()}`);
    }
  }
  if (created.courseId) {
    const response = await request.delete(`/api/v1/courses/${created.courseId}`, { headers });
    if (![200, 204, 404].includes(response.status())) {
      failures.push(`course ${created.courseId}: ${response.status()}`);
    }
  }
  expect(failures, "cleanup left rows behind").toEqual([]);
});

test("owner creates a course, prices it, opens a class on it and stops it once the class is gone", async ({
  page,
  request,
}) => {
  await loginAsOwner(page);

  // The nav entry lives in the Kho học liệu group, next to Lộ trình học.
  await page.goto("/courses");
  await expect(page.getByRole("heading", { name: "Khóa học", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "+ Tạo khóa học" }).click();
  const createDialog = page.getByRole("dialog", { name: "Tạo khóa học" });
  await createDialog.getByLabel("Mã khóa học").fill(RUN_CODE.toLowerCase());
  await createDialog.getByLabel("Tên khóa học").fill(COURSE_NAME);
  await createDialog.getByLabel("Môn học").fill("Toán");
  await createDialog.getByLabel("Cấp / trình độ").fill("Lớp 6");
  // Only an active course is offered when a class is created.
  await pickOption(page, "Trạng thái", "Đang hoạt động");
  await createDialog.getByLabel("Đơn giá / buổi (đ)").fill("150000");
  await createDialog.getByLabel("Tổng số buổi").fill("24");
  await createDialog.getByRole("button", { name: "Tạo" }).click();

  // Creating lands on the detail page.
  await expect(page).toHaveURL(/\/courses\/[0-9a-f-]+$/);
  created.courseId = page.url().split("/").pop();
  await expect(page.getByRole("heading", { name: COURSE_NAME })).toBeVisible();
  const main = page.getByRole("main");
  await expect(main.getByText(RUN_CODE, { exact: true })).toBeVisible();
  await expect(main.getByText("Đang hoạt động", { exact: true })).toBeVisible();
  const info = page.getByRole("tabpanel");
  await expect(info.getByText("150.000đ")).toBeVisible();

  // Edit keeps everything else as stored.
  await page.getByRole("button", { name: "Sửa khóa" }).click();
  const editDialog = page.getByRole("dialog", { name: "Sửa khóa học" });
  await editDialog.getByLabel("Tên khóa học").fill(COURSE_NAME_EDITED);
  await editDialog.getByRole("button", { name: "Lưu" }).click();
  await expect(page.getByText("Đã lưu khóa học")).toBeVisible();
  await expect(page.getByRole("heading", { name: COURSE_NAME_EDITED })).toBeVisible();
  await expect(info.getByText("150.000đ")).toBeVisible();

  // The duration and price edit in place on Thông tin chung.
  await info.getByRole("button", { name: "Chỉnh sửa" }).click();
  await info.getByLabel("Thời lượng buổi (phút)").fill("90");
  await info.getByRole("button", { name: "Lưu thay đổi" }).click();
  await expect(page.getByText("Đã lưu — áp dụng cho lớp mở mới")).toBeVisible();
  await expect(info.getByText("90 phút")).toBeVisible();

  // Tuition packs live in Thiết lập and save on every add.
  await info.getByRole("button", { name: /Gói học phí/ }).click();
  await expect(page).toHaveURL(/\?tab=setup$/);
  await page.getByRole("textbox", { name: "Tên gói" }).fill("Gói 12 buổi");
  await page.getByRole("spinbutton", { name: "Số buổi" }).fill("12");
  await page.getByRole("spinbutton", { name: "Giá gói" }).fill("1600000");
  await page.getByRole("button", { name: "+ Thêm gói" }).click();
  await expect(page.getByText("Đã thêm gói")).toBeVisible();
  await page.reload();
  await expect(main.getByText("Gói 12 buổi", { exact: true })).toBeVisible();
  // 12 × 150.000đ = 1.800.000đ, so a 1.600.000đ pack saves 11%.
  await expect(main.getByText("−11%")).toBeVisible();

  // A class opened on the course inherits its unit price.
  await page.goto("/students");
  await page.getByRole("button", { name: "+ Tạo lớp mới" }).click();
  const classDialog = page.getByRole("dialog");
  await classDialog.getByLabel("Tên lớp").fill(CLASS_NAME);
  await pickOption(page, "Khóa học", new RegExp(RUN_CODE));
  await expect(classDialog.getByLabel("Đơn giá / buổi (đ)")).toHaveValue("150.000");
  await classDialog.getByRole("button", { name: "T2" }).click();
  await classDialog.getByLabel("Giờ học khung 1").fill("18:00");
  await classDialog.getByLabel("Thời lượng (phút)").fill("90");
  await classDialog.getByRole("button", { name: "Tạo lớp" }).click();
  const classRow = page.getByRole("row").filter({ hasText: CLASS_NAME });
  await expect(classRow).toBeVisible();
  await expect(classRow.getByText("150.000 ₫/buổi")).toBeVisible();

  // The class list opens the class, whose chip links back to the course.
  await page.goto("/classes");
  await page.getByRole("link", { name: CLASS_NAME, exact: true }).click();
  await expect(page).toHaveURL(/\/classes\/[0-9a-f-]+$/);
  created.classId = page.url().split("/classes/")[1];
  await page.goto(`/classes/${created.classId}`);
  const chip = page.getByRole("link", { name: `Khóa: ${RUN_CODE} · ${COURSE_NAME_EDITED}` });
  await expect(chip).toBeVisible();
  await chip.click();
  await expect(page.getByRole("heading", { name: COURSE_NAME_EDITED })).toBeVisible();

  // The Lớp học tab lists the class and opens it.
  await page.getByRole("tab", { name: "Lớp học" }).click();
  const opsRow = page.getByRole("row").filter({ hasText: CLASS_NAME });
  await expect(opsRow).toBeVisible();

  // A course with an open class refuses to stop.
  await page.getByRole("button", { name: "Dừng hoạt động" }).click();
  await expect(page.getByText(/lớp đang mở — kết thúc lớp trước khi dừng/)).toBeVisible();

  // Once the class is gone the course stops and leaves the active list.
  const token = await apiLoginAsOwner(request);
  const removed = await request.delete(`/api/v1/classes/${created.classId}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect([200, 204]).toContain(removed.status());
  created.classId = undefined;
  await page.reload();
  await page.getByRole("button", { name: "Dừng hoạt động" }).click();
  await expect(page.getByText("Đã dừng hoạt động — không mở lớp mới từ khóa này")).toBeVisible();
  await expect(page.getByRole("button", { name: "Kích hoạt lại" })).toBeVisible();

  await page.goto("/courses?status=archived");
  await expect(page.getByRole("row").filter({ hasText: RUN_CODE })).toBeVisible();
});
