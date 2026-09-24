import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

import { apiLoginAsOwner, loginAsOwner } from "./helpers/auth.js";

// Dev-only seed: Cô Lan owns the center, so she creates and reads every class
// on /classes. The run's own class carries a distinctive weekday (Thứ 4) and
// morning start time and opens far in the future (phase "upcoming"), so the
// weekday/shift/phase filters each have an unambiguous in/out case without
// depending on what the seed happens to schedule. The second class is created
// straight through the "+ Lớp học" dialog to cover that path too.
const RUN_SUFFIX = Date.now().toString(36).toUpperCase();
const CLASS_NAME = `Lớp lọc e2e ${RUN_SUFFIX}`;
const NEW_CLASS_NAME = `Lớp mới e2e ${RUN_SUFFIX}`;

function isoDateInDays(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  return date.toISOString().slice(0, 10);
}

/** Ids the setup and the journey captured, so cleanup does not have to search for them. */
const created: { classId?: string; newClassId?: string } = {};

async function postJson<T>(
  request: APIRequestContext,
  headers: Record<string, string>,
  path: string,
  data: unknown,
): Promise<T> {
  const response = await request.post(path, { headers, data });
  expect(response.ok(), `${path} failed: ${response.status()} ${await response.text()}`).toBe(true);
  return ((await response.json()) as { data: T }).data;
}

async function pickOption(page: Page, combobox: string, option: RegExp | string) {
  await page.getByRole("combobox", { name: combobox }).click();
  await page.getByRole("listbox").getByRole("option", { name: option }).click();
}

test.beforeEach(async ({ request }) => {
  const token = await apiLoginAsOwner(request);
  const headers = { Authorization: `Bearer ${token}` };
  const klass = await postJson<{ id: string }>(request, headers, "/api/v1/classes", {
    name: CLASS_NAME,
    start_date: isoDateInDays(30),
    default_unit_price: 100000,
    schedules: [{ weekday: 3, start_time: "08:00", duration_min: 90 }],
  });
  created.classId = klass.id;
});

test.afterEach(async ({ request }) => {
  const token = await apiLoginAsOwner(request);
  const headers = { Authorization: `Bearer ${token}` };
  // Each step reports its own failure and lets the next one run, so one class
  // that would not delete still leaves the other attempt in the log instead
  // of silently leaking both rows.
  const failures: string[] = [];
  for (const [label, id] of [
    ["class", created.classId],
    ["new class", created.newClassId],
  ] as const) {
    if (!id) continue;
    const response = await request.delete(`/api/v1/classes/${id}`, { headers });
    if (![200, 204, 404].includes(response.status())) {
      failures.push(`${label} ${id}: ${response.status()}`);
    }
  }
  expect(failures, "cleanup left rows behind").toEqual([]);
});

test("owner filters the class list, opens a class and creates one from Danh sách lớp học", async ({
  page,
}) => {
  await loginAsOwner(page);

  await page.goto("/classes");
  await expect(page.getByRole("heading", { name: "Danh sách lớp học" })).toBeVisible();
  const row = page.getByRole("row").filter({ hasText: CLASS_NAME });
  await expect(row).toBeVisible();

  // The search box narrows to the run's own class by name.
  await page.getByRole("searchbox", { name: "Tìm lớp học" }).fill(CLASS_NAME);
  await expect(row).toBeVisible();
  await page.getByRole("searchbox", { name: "Tìm lớp học" }).fill("");

  // Weekday filter: the class only meets on Thứ 4.
  await pickOption(page, "Ngày học", "Thứ 4");
  await expect(row).toBeVisible();
  await pickOption(page, "Ngày học", "Thứ 2");
  await expect(row).toBeHidden();
  await pickOption(page, "Ngày học", "Tất cả các ngày");

  // Shift filter: 08:00 is a morning session.
  await pickOption(page, "Ca học", "Sáng");
  await expect(row).toBeVisible();
  await pickOption(page, "Ca học", "Tối");
  await expect(row).toBeHidden();
  await pickOption(page, "Ca học", "Tất cả ca");

  // Phase chips: the class opens 30 days out, so it is "Sắp khai giảng" and
  // never "Đã kết thúc".
  const chips = page.getByRole("radiogroup", { name: "Lọc theo trạng thái" });
  await chips.getByRole("radio", { name: "Sắp khai giảng" }).click();
  await expect(row).toBeVisible();
  await chips.getByRole("radio", { name: "Đã kết thúc" }).click();
  await expect(row).toBeHidden();
  await chips.getByRole("radio", { name: "Tất cả" }).click();
  await expect(row).toBeVisible();

  // Opening the detail from the list: click the row itself, not one of its
  // two links (the class name and the "Sửa" settings shortcut).
  await row.click({ position: { x: 5, y: 5 } });
  await expect(page).toHaveURL(new RegExp(`/classes/${created.classId}$`));
  await expect(page.getByRole("heading", { name: CLASS_NAME })).toBeVisible();

  // Creating a class from the list itself, through the "+ Lớp học" dialog.
  await page.goto("/classes");
  await page.getByRole("button", { name: "+ Lớp học" }).click();
  const classDialog = page.getByRole("dialog", { name: "Tạo lớp mới" });
  await classDialog.getByLabel("Tên lớp").fill(NEW_CLASS_NAME);
  await classDialog.getByRole("button", { name: "T2" }).click();
  await classDialog.getByLabel("Giờ học khung 1").fill("19:00");
  await classDialog.getByLabel("Đơn giá / buổi (đ)").fill("120000");
  await classDialog.getByRole("button", { name: "Tạo lớp" }).click();
  const newRow = page.getByRole("row").filter({ hasText: NEW_CLASS_NAME });
  await expect(newRow).toBeVisible();
  const settingsHref = await newRow.getByRole("link", { name: "Sửa" }).getAttribute("href");
  created.newClassId = /\/classes\/([0-9a-f-]+)\/settings/.exec(settingsHref ?? "")?.[1];
  expect(created.newClassId, "could not read the new class id off its settings link").toBeTruthy();
});
