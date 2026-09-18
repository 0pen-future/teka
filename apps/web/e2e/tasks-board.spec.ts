import { expect, test } from "@playwright/test";

import { apiLoginAsOwner, loginAsMember, loginAsOwner } from "./helpers/auth.js";
import {
  card,
  column,
  createTasks,
  deleteTask,
  findColumnId,
  findMemberId,
} from "./helpers/board.js";

test("task board flow: add a column, create, move, fold the column away, delete", async ({
  page,
}) => {
  // Timestamp suffix keeps this run's data unique: the config runs specs
  // sequentially against one seeded stack, and column names are unique per
  // center, so a reused database must not collide with an earlier run.
  const suffix = Date.now();
  const columnName = `E2E Cột ${suffix}`;
  const taskTitle = `E2E Việc ${suffix}`;

  await loginAsOwner(page);

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
  const taskCard = card(column(page, "Cần làm"), taskTitle);
  await taskCard.getByRole("button", { name: "Thao tác" }).click();
  await page.getByRole("menuitem", { name: columnName }).click();
  await expect(column(page, columnName).getByText(taskTitle)).toBeVisible();
  await expect(column(page, "Cần làm").getByText(taskTitle)).toHaveCount(0);
  // A menu move sends no neighbour, so the task lands at the top of the column.
  await expect(column(page, columnName).getByRole("option").first()).toContainText(taskTitle);

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
  await card(column(page, "Hoàn thành"), taskTitle).click();
  const detail = page.getByRole("dialog", { name: "Chi tiết công việc" });
  await detail.getByRole("button", { name: "Xoá" }).click();
  await page
    .getByRole("alertdialog", { name: "Xoá công việc này?" })
    .getByRole("button", { name: "Xoá" })
    .click();
  // The success toast repeats the title, so only the board proves the removal.
  await expect(column(page, "Hoàn thành").getByText(taskTitle)).toHaveCount(0);
});

test("task detail modal: dirty guard on close, delete undo, and an assignee's column-only edit", async ({
  page,
  browser,
}) => {
  const suffix = Date.now();

  await loginAsOwner(page);
  await page.goto("/tasks");
  const todo = column(page, "Cần làm");

  // Dirty guard: editing the title then trying to close (Escape) asks for
  // confirmation instead of silently discarding the change.
  const dirtyGuardTitle = `E2E Bỏ thay đổi ${suffix}`;
  await page.getByRole("button", { name: "Thêm việc vào Cần làm" }).click();
  const createDialog = page.getByRole("dialog", { name: "Tạo công việc" });
  await createDialog.getByLabel("Tiêu đề").fill(dirtyGuardTitle);
  await createDialog.getByRole("button", { name: "Lưu" }).click();
  await expect(createDialog).toBeHidden();

  await card(todo, dirtyGuardTitle).click();
  const detail = page.getByRole("dialog", { name: "Chi tiết công việc" });
  await detail.getByLabel("Tiêu đề").fill(`${dirtyGuardTitle} đã sửa`);
  await page.keyboard.press("Escape");
  const discardDialog = page.getByRole("alertdialog", { name: "Bỏ thay đổi chưa lưu?" });
  await expect(discardDialog).toBeVisible();
  await expect(discardDialog).toContainText(
    "Tiêu đề, mô tả hoặc thuộc tính bạn vừa sửa sẽ không được giữ.",
  );

  // "Tiếp tục sửa" cancels the discard and returns to the still-open, still-dirty form.
  await discardDialog.getByRole("button", { name: "Tiếp tục sửa" }).click();
  await expect(discardDialog).toBeHidden();
  await expect(detail).toBeVisible();
  await expect(detail.getByLabel("Tiêu đề")).toHaveValue(`${dirtyGuardTitle} đã sửa`);

  // Confirming the discard closes the modal and leaves the board's title untouched.
  await page.keyboard.press("Escape");
  await expect(discardDialog).toBeVisible();
  await discardDialog.getByRole("button", { name: "Bỏ thay đổi" }).click();
  await expect(detail).toBeHidden();
  await expect(card(todo, dirtyGuardTitle)).toBeVisible();
  await expect(todo.getByText(`${dirtyGuardTitle} đã sửa`)).toHaveCount(0);

  // Delete + undo: the toast's "Hoàn tác" restores the task, announced through
  // the page's live region (there are two `role="status"` regions on the
  // page, so this must filter rather than assert on the bare role).
  const restoreTitle = `E2E Khôi phục ${suffix}`;
  await page.getByRole("button", { name: "Thêm việc vào Cần làm" }).click();
  await createDialog.getByLabel("Tiêu đề").fill(restoreTitle);
  await createDialog.getByRole("button", { name: "Lưu" }).click();
  await expect(createDialog).toBeHidden();

  await card(todo, restoreTitle).click();
  await detail.getByRole("button", { name: "Xoá" }).click();
  await page
    .getByRole("alertdialog", { name: "Xoá công việc này?" })
    .getByRole("button", { name: "Xoá" })
    .click();
  await expect(detail).toBeHidden();
  await expect(todo.getByText(restoreTitle)).toHaveCount(0);
  await page.getByRole("button", { name: "Hoàn tác" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Đã khôi phục" })).toBeVisible();
  await expect(card(todo, restoreTitle)).toBeVisible();

  // D12: a task the owner created but assigned to Thầy Minh. Logged in as
  // that assignee (not the creator), only "Cột" may change — every other
  // field is disabled — and saving a new column moves the card.
  const token = await apiLoginAsOwner(page.request);
  const todoId = await findColumnId(page.request, token, "Cần làm");
  const minhId = await findMemberId(page.request, token, "Thầy Minh");
  const assigneeTitle = `E2E Chỉ đổi cột ${suffix}`;
  await createTasks(page.request, token, todoId, [{ title: assigneeTitle, assigneeId: minhId }]);

  const memberContext = await browser.newContext();
  const memberPage = await memberContext.newPage();
  try {
    await loginAsMember(memberPage);
    await memberPage.goto("/tasks");
    await card(column(memberPage, "Cần làm"), assigneeTitle).click();
    const memberDetail = memberPage.getByRole("dialog", { name: "Chi tiết công việc" });
    await expect(memberDetail).toBeVisible();
    await expect(memberDetail.getByLabel("Tiêu đề")).toBeDisabled();
    await expect(memberDetail.getByLabel("Người phụ trách")).toBeDisabled();
    await expect(memberDetail.getByLabel("Hạn")).toBeDisabled();
    await expect(memberDetail.getByLabel("Cột")).toBeEnabled();
    const saveButton = memberDetail.getByRole("button", { name: "Lưu" });
    await expect(saveButton).toBeDisabled();

    await memberDetail.getByLabel("Cột").click();
    await memberPage.getByRole("option", { name: "Đang làm" }).click();
    await expect(saveButton).toBeEnabled();
    await saveButton.click();
    await expect(memberDetail).toBeHidden();
    await expect(column(memberPage, "Đang làm").getByText(assigneeTitle)).toBeVisible();
    await expect(column(memberPage, "Cần làm").getByText(assigneeTitle)).toHaveCount(0);
  } finally {
    await memberContext.close();
  }

  // The move is server-side and center-wide, so the owner's own board (still
  // open in the primary page/tab) reflects it too.
  await page.reload();
  await expect(column(page, "Đang làm").getByText(assigneeTitle)).toBeVisible();
});

test("board keyboard navigation: roving tabindex, and [ ] moves the focused card", async ({
  page,
}) => {
  // Covers the automatable slice of the plan's manual a11y checklist (D-file
  // step 4): real DOM focus through the headless lib's roving tabindex
  // (`src/lib/kanban/use-kanban-keyboard.ts`), `[`/`]` moving the focused
  // card to an adjacent column and following it with focus, and the
  // resulting live-region announcement. Screen-reader announcement
  // wording/cadence itself still needs a manual VoiceOver/NVDA pass — see
  // the PR's manual checklist.
  //
  // Keyboard activation of the nested quick-done / menu buttons is covered
  // by the unit suite (`task-board-page.test.tsx`): `TaskCard`'s `onKeyDown`
  // only treats Enter/Space as "open" when the card itself is the target, so
  // a focused nested button keeps its native activation.
  const suffix = Date.now();
  const firstTitle = `E2E Phím A ${suffix}`;
  const secondTitle = `E2E Phím B ${suffix}`;

  await loginAsOwner(page);
  const token = await apiLoginAsOwner(page.request);
  const todoId = await findColumnId(page.request, token, "Cần làm");
  // Created in reverse so "Cần làm" shows [firstTitle, secondTitle] top to bottom.
  const [secondSeed, firstSeed] = await createTasks(page.request, token, todoId, [
    secondTitle,
    firstTitle,
  ]);

  await page.goto("/tasks");
  const todo = column(page, "Cần làm");
  const doing = column(page, "Đang làm");
  const firstCard = card(todo, firstTitle);
  const secondCard = card(todo, secondTitle);
  await expect(firstCard).toBeVisible();

  // Only the column's first card is a Tab stop until arrow-key focus moves
  // it; a real `.focus()` (not a synthetic click) exercises the same path a
  // keyboard/screen-reader user's Tab key would land on.
  await firstCard.focus();
  await expect(firstCard).toBeFocused();
  await expect(firstCard).toHaveAttribute("tabindex", "0");
  await expect(secondCard).toHaveAttribute("tabindex", "-1");

  await page.keyboard.press("ArrowDown");
  await expect(secondCard).toBeFocused();
  await expect(secondCard).toHaveAttribute("tabindex", "0");
  await expect(firstCard).toHaveAttribute("tabindex", "-1");

  // `]` moves the focused card to the next column, follows it with real
  // focus once the move settles, and announces it through the live region.
  await page.keyboard.press("]");
  await expect(card(doing, secondTitle)).toBeVisible();
  await expect(todo.getByText(secondTitle)).toHaveCount(0);
  await expect(card(doing, secondTitle)).toBeFocused();
  await expect(
    page.getByRole("status").filter({ hasText: `Đã chuyển việc sang cột Đang làm.` }),
  ).toBeVisible();

  // Clean up: the moved card would otherwise linger in "Đang làm" for later
  // specs sharing this database.
  await deleteTask(page.request, token, firstSeed.id);
  await deleteTask(page.request, token, secondSeed.id);
});
