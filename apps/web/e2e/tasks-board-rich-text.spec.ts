import { expect, test } from "@playwright/test";

import { apiLoginAsOwner, loginAsOwner } from "./helpers/auth.js";
import { card, column, findColumnId } from "./helpers/board.js";

test("a description written with the toolbar keeps bold, list and link after a save", async ({
  page,
}) => {
  const suffix = Date.now();
  const title = `E2E Mô tả ${suffix}`;

  await loginAsOwner(page);
  await page.goto("/tasks");
  await page.getByRole("button", { name: "Thêm việc vào Cần làm" }).click();
  const createDialog = page.getByRole("dialog", { name: "Tạo công việc" });
  await createDialog.getByLabel("Tiêu đề").fill(title);

  const editor = createDialog.getByRole("textbox", { name: "Mô tả" });
  await editor.click();
  await page.keyboard.type("Gọi phụ huynh ");
  await createDialog.getByRole("button", { name: "Đậm" }).click();
  await page.keyboard.type("trước thứ 6");
  await createDialog.getByRole("button", { name: "Đậm" }).click();
  await page.keyboard.press("Enter");
  await createDialog.getByRole("button", { name: "Danh sách chấm" }).click();
  await page.keyboard.type("lớp 6A");
  await page.keyboard.press("Enter");
  await page.keyboard.type("lớp 7B");
  await page.keyboard.press("Enter");
  await page.keyboard.press("Enter"); // leave the list
  await createDialog.getByRole("button", { name: "Liên kết" }).click();
  await createDialog.getByLabel("Địa chỉ liên kết").fill("https://teka.vn/lich");
  await createDialog.getByRole("button", { name: "Áp dụng" }).click();
  await expect(editor.locator("a[href='https://teka.vn/lich']")).toBeVisible();

  await createDialog.getByRole("button", { name: "Lưu" }).click();
  await expect(createDialog).toBeHidden();

  // Reopen: the owner may edit, so the dialog renders the editor with the
  // stored HTML; the marks must have survived the API round trip.
  await card(column(page, "Cần làm"), title).click();
  const detail = page.getByRole("dialog", { name: "Chi tiết công việc" });
  const stored = detail.getByRole("textbox", { name: "Mô tả" });
  await expect(stored.locator("strong", { hasText: "trước thứ 6" })).toBeVisible();
  await expect(stored.locator("ul li")).toHaveCount(2);
  await expect(stored.locator("a[href='https://teka.vn/lich'][rel~='noopener']")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(detail).toBeHidden();
});

test("a description posted straight to the API comes back without scripts or handlers", async ({
  page,
}) => {
  const suffix = Date.now();
  const title = `E2E XSS ${suffix}`;

  await loginAsOwner(page);
  const token = await apiLoginAsOwner(page.request);
  const todoId = await findColumnId(page.request, token, "Cần làm");
  const payload =
    '<p>an toàn</p><script>alert(1)</script><img src=x onerror="alert(1)"><p onclick="alert(2)"><a href="javascript:alert(3)">x</a></p>';
  const created = await page.request.post("/api/v1/tasks", {
    headers: { Authorization: `Bearer ${token}` },
    data: { title, column_id: todoId, description: payload },
  });
  expect(created.status()).toBe(201);
  const { data } = (await created.json()) as { data: { id: string; description: string } };

  const fetched = await page.request.get(`/api/v1/tasks/${data.id}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(fetched.ok()).toBe(true);
  const body = (await fetched.json()) as { data: { description: string } };
  expect(body.data.description).toContain("<p>an toàn</p>");
  expect(body.data.description).not.toMatch(/script|onerror|onclick|javascript:/i);

  // The board must render the stored HTML without ever opening a dialog.
  const dialogs: string[] = [];
  page.on("dialog", (dialog) => {
    dialogs.push(dialog.message());
    void dialog.dismiss();
  });
  await page.goto("/tasks");
  await card(column(page, "Cần làm"), title).click();
  const detail = page.getByRole("dialog", { name: "Chi tiết công việc" });
  await expect(detail.getByRole("textbox", { name: "Mô tả" })).toContainText("an toàn");
  expect(dialogs).toEqual([]);
});
