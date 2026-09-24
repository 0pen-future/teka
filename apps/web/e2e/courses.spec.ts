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

test("owner creates a course, prices it, opens a class on it and stops recruiting", async ({
  page,
}) => {
  await loginAsOwner(page);

  // The nav entry lives in the Giảng dạy group; Kho học liệu now has its own sidebar group.
  await page.goto("/courses");
  await expect(page.getByRole("heading", { name: "Danh mục khóa học" })).toBeVisible();

  await page.getByRole("button", { name: "Tạo khóa học" }).click();
  const createDialog = page.getByRole("dialog", { name: "Tạo khóa học" });
  await createDialog.getByLabel("Mã khóa học").fill(RUN_CODE.toLowerCase());
  await createDialog.getByLabel("Tên khóa học").fill(COURSE_NAME);
  await createDialog.getByLabel("Môn học").fill("Toán");
  await createDialog.getByLabel("Cấp / trình độ").fill("Lớp 6");
  // Only an active course is offered when a class is created.
  await pickOption(page, "Trạng thái", "Đang mở");
  await createDialog.getByLabel("Đơn giá / buổi (đ)").fill("150000");
  await createDialog.getByLabel("Tổng số buổi").fill("24");
  await createDialog.getByRole("button", { name: "Tạo" }).click();

  // Creating lands on the detail page.
  await expect(page).toHaveURL(/\/courses\/[0-9a-f-]+$/);
  created.courseId = page.url().split("/").pop();
  await expect(page.getByRole("heading", { name: COURSE_NAME })).toBeVisible();
  await expect(page.getByText(`Mã: ${RUN_CODE}`)).toBeVisible();
  // The sidebar also says "Đang mở" about the center, so scope to the page body.
  const main = page.getByRole("main");
  await expect(main.getByText("Đang mở")).toBeVisible();
  const info = page.getByRole("tabpanel");
  await expect(info.getByText("150.000 ₫")).toBeVisible();
  await expect(info.getByText("24 buổi")).toBeVisible();

  // Edit keeps everything else as stored.
  await page.getByRole("button", { name: "Sửa" }).click();
  const editDialog = page.getByRole("dialog", { name: "Sửa khóa học" });
  await editDialog.getByLabel("Tên khóa học").fill(COURSE_NAME_EDITED);
  await editDialog.getByRole("button", { name: "Lưu" }).click();
  await expect(page.getByText("Đã lưu khóa học")).toBeVisible();
  await expect(page.getByRole("heading", { name: COURSE_NAME_EDITED })).toBeVisible();
  await expect(info.getByText("150.000 ₫")).toBeVisible();

  // Tuition packs are saved wholesale in display order.
  await page.getByRole("tab", { name: "Gói học phí" }).click();
  const packs = page.getByRole("region", { name: "Gói học phí" });
  await packs.getByRole("button", { name: "Thêm gói" }).click();
  await packs.getByRole("textbox", { name: "Tên gói 1" }).fill("Gói 12 buổi");
  await packs.getByRole("spinbutton", { name: "Số buổi 1" }).fill("12");
  await packs.getByRole("spinbutton", { name: "Giá 1" }).fill("1600000");
  await packs.getByRole("button", { name: "Thêm gói" }).click();
  await packs.getByRole("textbox", { name: "Tên gói 2" }).fill("Gói 24 buổi");
  await packs.getByRole("spinbutton", { name: "Số buổi 2" }).fill("24");
  await packs.getByRole("spinbutton", { name: "Giá 2" }).fill("3000000");
  await packs.getByRole("button", { name: "Chuyển lên gói 2" }).click();
  await packs.getByRole("button", { name: "Lưu gói học phí" }).click();
  await expect(page.getByText("Đã lưu gói học phí")).toBeVisible();
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Tên gói 1" })).toHaveValue("Gói 24 buổi");
  await expect(page.getByRole("textbox", { name: "Tên gói 2" })).toHaveValue("Gói 12 buổi");

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
  await classRow.getByRole("link", { name: "⚙ Cài đặt" }).click();
  await expect(page).toHaveURL(/\/classes\/[0-9a-f-]+\/settings$/);
  created.classId = /\/classes\/([0-9a-f-]+)/.exec(page.url())?.[1];

  // The class list shows the course code; the class header links back to it.
  await page.goto("/classes");
  await expect(
    page.getByRole("row").filter({ hasText: CLASS_NAME }).getByText(RUN_CODE, { exact: true }),
  ).toBeVisible();
  await page.goto(`/classes/${created.classId}`);
  const chip = page.getByRole("link", { name: `Khóa: ${RUN_CODE} · ${COURSE_NAME_EDITED}` });
  await expect(chip).toBeVisible();
  await chip.click();
  await expect(page.getByRole("heading", { name: COURSE_NAME_EDITED })).toBeVisible();

  // The Vận hành tab lists the class.
  await page.getByRole("tab", { name: "Vận hành" }).click();
  const opsRow = page.getByRole("row").filter({ hasText: CLASS_NAME });
  await expect(opsRow.getByRole("link", { name: CLASS_NAME })).toHaveAttribute(
    "href",
    `/classes/${created.classId}`,
  );

  // Stop recruiting: the course leaves the class picker, the class keeps running.
  await page.getByRole("button", { name: "Ngừng tuyển" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Ngừng tuyển" }).click();
  await expect(page.getByText(`Đã ngừng tuyển khóa ${RUN_CODE}`)).toBeVisible();
  await expect(page.getByRole("button", { name: "Ngừng tuyển" })).toBeHidden();
  await expect(main.getByText("Ngừng tuyển", { exact: true })).toBeVisible();

  await page.goto("/courses?status=archived");
  await expect(page.getByRole("row").filter({ hasText: RUN_CODE })).toBeVisible();
});
