import { expect, test, type Page } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const OWNER_PHONE = "0901000001";
const OWNER_PASSWORD = "lan-password";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(OWNER_PHONE);
  await page.getByLabel("Mật khẩu").fill(OWNER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();
}

function column(page: Page, name: string) {
  return page.getByRole("listbox", { name });
}

test("task board flow: add a column, create, move, fold the column away, delete", async ({
  page,
}) => {
  // Timestamp suffix keeps this run's data unique: the config runs specs
  // sequentially against one seeded stack, and column names are unique per
  // center, so a reused database must not collide with an earlier run.
  const suffix = Date.now();
  const columnName = `E2E Cột ${suffix}`;
  const taskTitle = `E2E Việc ${suffix}`;

  await login(page);

  // 1. The owner lands on the seeded default columns.
  await page.goto("/tasks");
  await expect(column(page, "Cần làm")).toBeVisible();
  await expect(column(page, "Hoàn thành")).toBeVisible();

  // 2. Add a column through the board settings modal (owner has manage_board).
  await page.getByRole("button", { name: "Cấu hình cột" }).click();
  const settings = page.getByRole("dialog", { name: "Cấu hình cột bảng công việc" });
  await settings.getByLabel("Tên cột mới").fill(columnName);
  await settings.getByRole("button", { name: "Thêm cột" }).click();
  await expect(settings.getByLabel(`Tên cột ${columnName}`)).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(settings).toBeHidden();
  await expect(column(page, columnName)).toBeVisible();

  // 3. Create a task in "Cần làm" through the column's add button.
  await page.getByRole("button", { name: "Thêm việc vào Cần làm" }).click();
  const createDialog = page.getByRole("dialog", { name: "Tạo công việc" });
  await createDialog.getByLabel("Tiêu đề").fill(taskTitle);
  await createDialog.getByRole("button", { name: "Lưu" }).click();
  await expect(createDialog).toBeHidden();
  await expect(column(page, "Cần làm").getByText(taskTitle)).toBeVisible();

  // 4. Move it to the new column through the card's move menu.
  const card = column(page, "Cần làm").getByRole("option", { name: new RegExp(taskTitle) });
  await card.getByRole("button", { name: "Chuyển cột" }).click();
  await page.getByRole("menuitem", { name: columnName }).click();
  await expect(column(page, columnName).getByText(taskTitle)).toBeVisible();
  await expect(column(page, "Cần làm").getByText(taskTitle)).toHaveCount(0);

  // 5. Delete the new column: it still holds the task, so a destination is
  // required and the task must land there.
  await page.getByRole("button", { name: "Cấu hình cột" }).click();
  await settings.getByRole("button", { name: `Xoá cột ${columnName}` }).click();
  const deleteColumnDialog = page.getByRole("dialog", { name: `Xoá cột "${columnName}"?` });
  const confirmDelete = deleteColumnDialog.getByRole("button", { name: "Xoá cột" });
  await expect(confirmDelete).toBeDisabled();
  await deleteColumnDialog.getByRole("combobox", { name: "Chuyển việc sang cột" }).click();
  await page.getByRole("option", { name: "Hoàn thành" }).click();
  await confirmDelete.click();
  await expect(deleteColumnDialog).toBeHidden();
  await expect(settings.getByLabel(`Tên cột ${columnName}`)).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(settings).toBeHidden();
  await expect(column(page, columnName)).toHaveCount(0);
  await expect(column(page, "Hoàn thành").getByText(taskTitle)).toBeVisible();

  // 6. Delete the task from its detail form.
  await page.getByText(taskTitle).click();
  const detail = page.getByRole("dialog", { name: "Chi tiết công việc" });
  await detail.getByRole("button", { name: "Xoá" }).click();
  await page
    .getByRole("dialog", { name: "Xoá công việc này?" })
    .getByRole("button", { name: "Xoá" })
    .click();
  // The success toast repeats the title, so only the board proves the removal.
  await expect(column(page, "Hoàn thành").getByText(taskTitle)).toHaveCount(0);
});
