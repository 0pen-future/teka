import { expect, test, type APIRequestContext } from "@playwright/test";

import { apiLoginAsOwner, loginAsOwner } from "./helpers/auth.js";

// Dev-only seed: Cô Lan owns the center, so she applies programs, chats in
// any class and reads the audit trail. The run builds its own published
// template and its own class over the API, so no seeded class gets its
// classbook renamed, and the afterEach removes all of it again: a template
// refuses to be deleted while a class still follows one of its versions.
const RUN_CODE = `E2E-${Date.now().toString(36).toUpperCase()}`.slice(0, 20);
const TEMPLATE_NAME = `Chương trình lớp ${RUN_CODE}`;
const CLASS_NAME = `Lớp chương trình ${RUN_CODE}`;
const LESSONS = ["Số tự nhiên", "Phân số"] as const;
const MESSAGE = `Ghi chú nội bộ ${RUN_CODE}`;

/** Ids the setup captured, so cleanup does not have to search for them. */
const created: { templateId?: string; classId?: string } = {};

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

test.beforeEach(async ({ request }) => {
  const token = await apiLoginAsOwner(request);
  const headers = { Authorization: `Bearer ${token}` };

  const template = await postJson<{ id: string }>(request, headers, "/api/v1/library/templates", {
    code: RUN_CODE,
    name: TEMPLATE_NAME,
    subject: "Toán",
    level: "Lớp 6",
    description: null,
  });
  created.templateId = template.id;

  // Creating a template opens an empty v1 draft; fill it and publish it.
  const versions = await request.get(`/api/v1/library/templates/${template.id}/versions`, {
    headers,
  });
  const draft = ((await versions.json()) as { data: { id: string }[] }).data[0];
  expect(draft, "the new template has no draft version").toBeTruthy();
  for (const title of LESSONS) {
    await postJson(request, headers, `/api/v1/library/versions/${draft.id}/lessons`, {
      title,
      objectives: null,
      duration_min: 90,
      homework_note: null,
    });
  }
  await postJson(request, headers, `/api/v1/library/versions/${draft.id}/publish`, {});

  const klass = await postJson<{ id: string }>(request, headers, "/api/v1/classes", {
    name: CLASS_NAME,
    start_date: new Date().toISOString().slice(0, 10),
    default_unit_price: 100000,
    schedules: [{ weekday: 1, start_time: "18:00", duration_min: 90 }],
  });
  created.classId = klass.id;
});

test.afterEach(async ({ request }) => {
  const token = await apiLoginAsOwner(request);
  const headers = { Authorization: `Bearer ${token}` };
  // Each step reports its own failure and lets the next one run, so a class
  // that would not delete still leaves the template attempt (and its 409) in
  // the log instead of silently leaking both rows.
  const failures: string[] = [];
  const steps: [string, string][] = [];
  if (created.classId) {
    steps.push([
      `program of class ${created.classId}`,
      `/api/v1/classes/${created.classId}/program`,
    ]);
    steps.push([`class ${created.classId}`, `/api/v1/classes/${created.classId}`]);
  }
  if (created.templateId) {
    steps.push([
      `template ${created.templateId}`,
      `/api/v1/library/templates/${created.templateId}`,
    ]);
  }
  for (const [label, path] of steps) {
    const response = await request.delete(path, { headers });
    if (![200, 204, 404].includes(response.status())) {
      failures.push(`${label}: ${response.status()}`);
    }
  }
  expect(failures, "cleanup left rows behind").toEqual([]);
});

test("owner applies a published template to a class, reads it through the tabs, chats and removes it", async ({
  page,
}) => {
  await loginAsOwner(page);
  await page.goto(`/classes/${created.classId}`);
  await expect(page.getByRole("heading", { name: CLASS_NAME })).toBeVisible();

  // Apply: pick the run's template; its only published version preselects.
  const programCard = page.getByRole("region", { name: "Chương trình học" });
  await expect(programCard.getByText("Lớp chưa áp dụng chương trình mẫu.")).toBeVisible();
  await programCard.getByRole("button", { name: "Thiết lập chương trình" }).click();
  await programCard.getByRole("combobox", { name: "Chương trình mẫu" }).click();
  await page
    .getByRole("listbox")
    .getByRole("option", { name: new RegExp(RUN_CODE) })
    .click();
  await expect(programCard.getByRole("combobox", { name: "Phiên bản" })).toHaveText(
    /v1 · Đã phát hành/,
  );
  await programCard.getByRole("button", { name: "Áp dụng" }).click();
  await expect(page.getByText("Đã áp dụng chương trình mẫu")).toBeVisible();
  await expect(programCard.getByText(`${TEMPLATE_NAME} · v1 · 2 buổi`)).toBeVisible();

  // The lineage card records nothing yet, but the owner can read the change log.
  // Audit capture is async (batched flush), so reload until the row lands
  // instead of sleeping a fixed amount.
  const lineageCard = page.getByRole("region", { name: "Lịch sử lớp" });
  await expect(lineageCard.getByText("Lớp không tách từ lớp nào.")).toBeVisible();
  await expect(async () => {
    await page.reload();
    await lineageCard.getByRole("button", { name: "Lịch sử thay đổi" }).click();
    await expect(lineageCard.getByText("class_program.apply")).toBeVisible({ timeout: 2_000 });
  }).toPass({ timeout: 15_000 });

  // The read-through tabs group by the template's lessons.
  await page.getByRole("tab", { name: "Bài tập" }).click();
  await expect(page.getByRole("heading", { name: `Buổi 1 · ${LESSONS[0]}` })).toBeVisible();
  await expect(page.getByRole("heading", { name: `Buổi 2 · ${LESSONS[1]}` })).toBeVisible();
  await expect(page.getByText("Chưa gắn bài tập").first()).toBeVisible();

  await page.getByRole("tab", { name: "Tài liệu" }).click();
  await expect(page.getByRole("switch", { name: "Hiện tất cả" })).toBeVisible();
  await expect(page.getByRole("heading", { name: `Buổi 1 · ${LESSONS[0]}` })).toBeVisible();

  // Chat: the owner posts and then withdraws an internal note.
  await page.getByRole("tab", { name: "Chat" }).click();
  await expect(page.getByText("Tin nhắn nội bộ, không đồng bộ Zalo")).toBeVisible();
  await expect(page.getByText("Chưa có tin nhắn nào.")).toBeVisible();
  await page.getByRole("textbox", { name: "Nội dung tin nhắn" }).fill(MESSAGE);
  await page.getByRole("button", { name: "Gửi" }).click();
  const message = page.getByRole("listitem").filter({ hasText: MESSAGE });
  await expect(message).toBeVisible();
  await expect(message).toContainText("Cô Lan");
  await message.getByRole("button", { name: /^Xoá tin nhắn của Cô Lan/ }).click();
  const deleteDialog = page.getByRole("dialog");
  await expect(deleteDialog).toContainText("Xoá tin nhắn này?");
  await deleteDialog.getByRole("button", { name: "Xoá", exact: true }).click();
  await expect(page.getByText("Chưa có tin nhắn nào.")).toBeVisible();

  // Remove: the confirmation names what stays, and the card empties again.
  await page.getByRole("tab", { name: "Thông tin" }).click();
  await programCard.getByRole("button", { name: "Gỡ chương trình" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Sổ đầu bài và giáo án giữ nguyên");
  await dialog.getByRole("button", { name: "Gỡ", exact: true }).click();
  await expect(page.getByText("Đã gỡ chương trình mẫu")).toBeVisible();
  await expect(programCard.getByText("Lớp chưa áp dụng chương trình mẫu.")).toBeVisible();
});
