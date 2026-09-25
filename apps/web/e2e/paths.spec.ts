import { expect, test, type Page } from "@playwright/test";

import { apiLoginAsOwner, loginAsOwner } from "./helpers/auth.js";

// Dev-only seed: Cô Lan owns the center, so she manages paths and courses.
// The run creates its own active course over the API so the stage picker has
// something real to offer without touching a seeded course; the path and
// stage live entirely inside this run's code/name, so afterEach can remove
// both without racing any other spec's data.
const RUN_SUFFIX = Date.now().toString(36).toUpperCase();
const PATH_CODE = `LT-${RUN_SUFFIX}`.slice(0, 20);
const PATH_NAME = `Lộ trình e2e ${RUN_SUFFIX}`;
const PATH_DESCRIPTION_EDITED = `Mô tả đã sửa ${RUN_SUFFIX}`;
const COURSE_CODE = `KH-${RUN_SUFFIX}`.slice(0, 20);
const COURSE_NAME = `Khóa lộ trình ${RUN_SUFFIX}`;
const STAGE_NAME = `Nền tảng ${RUN_SUFFIX}`;
const STAGE_NAME_EDITED = `${STAGE_NAME} (sửa)`;
const STAGE_GOAL_EDITED = `Mục tiêu đã sửa ${RUN_SUFFIX}`;

/** Ids the setup and the journey captured, so cleanup does not have to search for them. */
const created: { courseId?: string; pathId?: string } = {};

async function pickOption(page: Page, combobox: string, option: RegExp | string) {
  await page.getByRole("combobox", { name: combobox }).click();
  await page.getByRole("listbox").getByRole("option", { name: option }).click();
}

test.beforeEach(async ({ request }) => {
  const token = await apiLoginAsOwner(request);
  const headers = { Authorization: `Bearer ${token}` };
  const response = await request.post("/api/v1/courses", {
    headers,
    data: { code: COURSE_CODE, name: COURSE_NAME, status: "active" },
  });
  expect(response.ok(), `course setup failed: ${response.status()} ${await response.text()}`).toBe(
    true,
  );
  const course = ((await response.json()) as { data: { id: string } }).data;
  created.courseId = course.id;
});

test.afterEach(async ({ request }) => {
  const token = await apiLoginAsOwner(request);
  const headers = { Authorization: `Bearer ${token}` };
  // Each step reports its own failure and lets the next one run, so a course
  // that would not delete still leaves the path attempt (and its status) in
  // the log instead of silently leaking both rows.
  const failures: string[] = [];
  // The list page creates paths without navigating, so find this run's path by code.
  if (!created.pathId) {
    const list = await request.get(`/api/v1/paths?per_page=100&q=${PATH_CODE}`, { headers });
    const rows = list.ok()
      ? ((await list.json()) as { data: { id: string; code: string }[] }).data
      : [];
    created.pathId = rows.find((row) => row.code === PATH_CODE)?.id;
  }
  if (created.pathId) {
    const response = await request.delete(`/api/v1/paths/${created.pathId}`, { headers });
    if (![200, 204, 404].includes(response.status())) {
      failures.push(`path ${created.pathId}: ${response.status()}`);
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

test("owner builds a learning path, stages it, picks a course and tears the stage down", async ({
  page,
}) => {
  await loginAsOwner(page);

  await page.goto("/paths");
  await expect(page.getByRole("heading", { name: "Lộ trình học" })).toBeVisible();

  await page.getByRole("button", { name: "+ Tạo lộ trình" }).click();
  const createDialog = page.getByRole("dialog", { name: "Tạo lộ trình" });
  await createDialog.getByLabel("Mã lộ trình").fill(PATH_CODE);
  await createDialog.getByLabel("Tên lộ trình").fill(PATH_NAME);
  await createDialog.getByRole("button", { name: "Tạo", exact: true }).click();

  // Creating stays on the list and opens the new row.
  await expect(page.getByText(`Đã tạo lộ trình ${PATH_CODE}`)).toBeVisible();
  const row = page.getByRole("row").filter({ hasText: PATH_NAME }).first();
  await expect(row.getByText("Nháp")).toBeVisible();
  await expect(page.getByRole("button", { name: `Thu gọn ${PATH_NAME}` })).toHaveAttribute(
    "aria-expanded",
    "true",
  );

  // Editing keeps the code and adds a description; the status can move
  // straight from draft to active from the same form.
  await page.getByRole("button", { name: `Sửa ${PATH_NAME}` }).click();
  const editDialog = page.getByRole("dialog", { name: "Sửa lộ trình" });
  await expect(editDialog.getByLabel("Mã lộ trình")).toHaveValue(PATH_CODE);
  await editDialog.getByLabel("Mô tả").fill(PATH_DESCRIPTION_EDITED);
  await pickOption(page, "Trạng thái", "Đang hoạt động");
  await editDialog.getByRole("button", { name: "Lưu" }).click();
  await expect(page.getByText("Đã lưu lộ trình")).toBeVisible();
  await expect(row.getByText("Đang hoạt động")).toBeVisible();

  // A new stage starts empty, offering the run's own active course in the picker.
  await page.getByRole("button", { name: "+ Thêm chặng" }).click();
  const stageDialog = page.getByRole("dialog", { name: "Thêm chặng" });
  await stageDialog.getByLabel("Tên chặng").fill(STAGE_NAME);
  await stageDialog.getByRole("button", { name: "Thêm", exact: true }).click();

  const stage = page.getByRole("group", { name: `Chặng 1: ${STAGE_NAME}` });
  await expect(stage).toBeVisible();

  await stage.getByRole("button", { name: `Thêm khóa vào ${STAGE_NAME}` }).click();
  await pickOption(page, `Thêm khóa học vào ${STAGE_NAME}`, new RegExp(COURSE_CODE));
  const coursesDialog = page.getByRole("dialog", { name: `Khóa học của chặng ${STAGE_NAME}` });
  await expect(coursesDialog.getByText(`${COURSE_CODE} · ${COURSE_NAME}`)).toBeVisible();
  await coursesDialog.getByRole("button", { name: "Đóng" }).last().click();
  await expect(stage.getByText(`${COURSE_CODE} · ${COURSE_NAME}`)).toBeVisible();

  // Renaming the stage keeps its course list untouched.
  await stage.getByRole("button", { name: STAGE_NAME, exact: true }).click();
  const renameDialog = page.getByRole("dialog", { name: "Đổi tên chặng" });
  await renameDialog.getByLabel("Tên chặng").fill(STAGE_NAME_EDITED);
  await renameDialog.getByLabel("Mục tiêu").fill(STAGE_GOAL_EDITED);
  await renameDialog.getByRole("button", { name: "Lưu" }).click();
  const stageAfterEdit = page.getByRole("group", { name: `Chặng 1: ${STAGE_NAME_EDITED}` });
  await expect(stageAfterEdit).toBeVisible();
  await expect(stageAfterEdit.getByText(`${COURSE_CODE} · ${COURSE_NAME}`)).toBeVisible();

  // Removing the course leaves an empty stage; deleting the stage then leaves
  // an empty path. The path itself is removed by afterEach.
  await stageAfterEdit.getByRole("button", { name: `Thêm khóa vào ${STAGE_NAME_EDITED}` }).click();
  const coursesAgain = page.getByRole("dialog", {
    name: `Khóa học của chặng ${STAGE_NAME_EDITED}`,
  });
  await coursesAgain.getByRole("button", { name: `Gỡ ${COURSE_NAME}` }).click();
  await expect(coursesAgain.getByText("Chưa gắn khóa học nào.")).toBeVisible();
  await coursesAgain.getByRole("button", { name: "Đóng" }).last().click();

  await stageAfterEdit.getByRole("button", { name: `Xoá chặng ${STAGE_NAME_EDITED}` }).click();
  const stageDeleteDialog = page.getByRole("dialog", { name: `Xoá chặng "${STAGE_NAME_EDITED}"?` });
  await stageDeleteDialog.getByRole("button", { name: "Xoá chặng", exact: true }).click();
  await expect(page.getByRole("group", { name: /Chặng 1:/ })).toHaveCount(0);
});
