import { expect, test } from "@playwright/test";

import { apiLoginAsOwner, loginAsMember, loginAsOwner } from "./helpers/auth.js";
import {
  card,
  column,
  createTasks,
  deleteTask,
  filterChip,
  findColumnId,
  findMemberId,
  moveTask,
  quickDone,
  topCardTitles,
  waitForBoard,
} from "./helpers/board.js";
import { dragTo } from "./helpers/drag.js";

/** `date`'s own calendar day as `YYYY-MM-DD`, read via local getters — mirrors `lib/due-state.ts#localIsoDate`. */
function isoDate(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

test("board filter bar dispatches: status/teacher chips, quick-done, collapse, color, and the no-view_all board", async ({
  page,
  request,
  browser,
}) => {
  // This single test covers 8 cases end to end (11 seeded tasks over the API,
  // two UI logins, a drag, two reloads, a column-color change, and an extra
  // board fetch to clear the teacher facet before asserting "mine") well past
  // the suite's 30s default.
  test.setTimeout(90_000);

  // Timestamp suffix keeps this run's data unique on a reused database.
  const suffix = Date.now();
  const today = new Date();
  const todayIso = isoDate(today);
  const yesterdayIso = isoDate(
    new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1),
  );

  const overdueTitle = `E2E Điều phối Quá hạn ${suffix}`;
  const todayTitle = `E2E Điều phối Hôm nay ${suffix}`;
  const noDueTitle = `E2E Điều phối Không hạn ${suffix}`;
  const minhATitle = `E2E Điều phối Việc Minh A ${suffix}`;
  const minhBTitle = `E2E Điều phối Việc Minh B ${suffix}`;
  const myDoneTitle = `E2E Điều phối Của tôi xong ${suffix}`;
  const quickDoneTitle = `E2E Điều phối Nhanh xong ${suffix}`;
  const [mineA, mineB, mineC, otherX] = ["A", "B", "C", "X"].map(
    (letter) => `E2E Điều phối Kéo ${letter} ${suffix}`,
  );

  // Seed through the API: one owner token drives every fixture, so the UI
  // steps below only ever exercise the filter/quick-done/collapse/color
  // behavior, not task creation.
  const token = await apiLoginAsOwner(request);
  const todoId = await findColumnId(request, token, "Cần làm");
  const doingId = await findColumnId(request, token, "Đang làm");
  const doneId = await findColumnId(request, token, "Hoàn thành");
  const ownerId = await findMemberId(request, token, "Cô Lan");
  const minhId = await findMemberId(request, token, "Thầy Minh");

  const todoSeeds = await createTasks(request, token, todoId, [
    { title: overdueTitle, dueOn: yesterdayIso },
    { title: todayTitle, dueOn: todayIso },
    { title: noDueTitle },
    { title: minhATitle, assigneeId: minhId },
    { title: minhBTitle, assigneeId: minhId },
    { title: quickDoneTitle },
  ]);
  const [myDone] = await createTasks(request, token, todoId, [
    { title: myDoneTitle, assigneeId: ownerId },
  ]);
  await moveTask(request, token, myDone.id, doneId);
  // Created in reverse so "Đang làm" shows [mineA, otherX, mineB, mineC] top to bottom.
  const doingSeeds = await createTasks(request, token, doingId, [
    { title: mineC, assigneeId: ownerId },
    { title: mineB, assigneeId: ownerId },
    { title: otherX },
    { title: mineA, assigneeId: ownerId },
  ]);

  await loginAsOwner(page);
  await page.goto("/tasks");
  const todo = column(page, "Cần làm");
  const doing = column(page, "Đang làm");
  const done = column(page, "Hoàn thành");
  await expect(card(todo, quickDoneTitle)).toBeVisible();

  // 1. "Quá hạn" chip: URL + request query, only the overdue task stays
  // visible, and a reload keeps the chip pressed.
  const overdueChip = filterChip(page, "Quá hạn");
  await expect(overdueChip).toHaveAccessibleName("Quá hạn 1");
  const overdueResponse = waitForBoard(page, { filter: "overdue" });
  await overdueChip.click();
  const overdueUrl = (await overdueResponse).url();
  expect(overdueUrl).toContain("filter=overdue");
  expect(overdueUrl).toContain(`today=${todayIso}`);
  await expect(page).toHaveURL(/[?&]filter=overdue(&|$)/);
  await expect(card(todo, overdueTitle)).toBeVisible();
  await expect(todo.getByText(todayTitle)).toHaveCount(0);
  await expect(todo.getByText(noDueTitle)).toHaveCount(0);
  await expect(todo.getByText(quickDoneTitle)).toHaveCount(0);
  await page.reload();
  await expect(page).toHaveURL(/[?&]filter=overdue(&|$)/);
  await expect(card(todo, overdueTitle)).toBeVisible();
  await expect(filterChip(page, "Quá hạn")).toHaveAttribute("aria-checked", "true");

  // 2. Teacher chip "Thầy Minh": label carries the seeded open-task count,
  // clicking it sets `?assignee=`.
  // "all" is the default filter, dropped from the query entirely (see
  // `getBoard`), so this wait only pins down that a fresh board request landed.
  const allChip = filterChip(page, "Tất cả");
  const allResponse = waitForBoard(page);
  await allChip.click();
  await allResponse;
  const minhChip = filterChip(page, "Thầy Minh");
  await expect(minhChip).toHaveAccessibleName("Thầy Minh 2");
  const minhResponse = waitForBoard(page, { assignee: minhId });
  await minhChip.click();
  await minhResponse;
  await expect(page).toHaveURL(new RegExp(`[?&]assignee=${minhId}(&|$)`));
  await expect(card(todo, minhATitle)).toBeVisible();
  await expect(card(todo, minhBTitle)).toBeVisible();
  await expect(todo.getByText(overdueTitle)).toHaveCount(0);

  // Clear the teacher facet before testing "mine": status and assignee are
  // independent filters that AND together (D3), so leaving Minh's chip
  // pressed would have his assignee override "mine"'s implied self-assignee.
  // This state ("all", no assignee) was already fetched by `allChip` above,
  // so TanStack Query serves it from cache with no new board request — assert
  // the URL/DOM settle instead of waiting on a network round trip.
  await minhChip.click();
  await expect(page).not.toHaveURL(/[?&]assignee=/);
  await expect(minhChip).toHaveAttribute("aria-checked", "false");

  // 3. "Của tôi" shows the caller's own DONE task in "Hoàn thành".
  const mineChip = filterChip(page, "Của tôi");
  const mineResponse = waitForBoard(page, { filter: "mine" });
  await mineChip.click();
  await mineResponse;
  await expect(page).toHaveURL(/[?&]filter=mine(&|$)/);
  await expect(card(done, myDoneTitle)).toBeVisible();
  await expect(todo.getByText(noDueTitle)).toHaveCount(0);

  // Clean up: this spec is the only one that ever parks a task in "Hoàn
  // thành", and `tasks-board-dnd.spec.ts` (which runs right after this file
  // alphabetically, against the same seeded database) assumes it starts that
  // column empty. Delete over the API now that this case's assertions are
  // done, rather than leaving the card there for the rest of the suite. (The
  // rest of this spec's seed data is deleted at the very end, once every case
  // that still needs it has run — see the cleanup after case 8.)
  await deleteTask(request, token, myDone.id);

  // 4. Drag while filtered drops after the visible predecessor (D9); the
  // full order survives clearing the filter. The "mine" filter from step 3
  // is still active, so "Đang làm" shows only [mineA, mineB, mineC].
  await expect.poll(() => topCardTitles(doing, 3)).toEqual([mineA, mineB, mineC]);
  await dragTo(page, card(doing, mineC), card(doing, mineB));
  await expect.poll(() => topCardTitles(doing, 3)).toEqual([mineA, mineC, mineB]);
  const clearedResponse = waitForBoard(page);
  await page.getByRole("button", { name: "Xoá lọc" }).click();
  await clearedResponse;
  await expect(page).not.toHaveURL(/[?&]filter=/);
  await expect.poll(() => topCardTitles(doing, 4)).toEqual([mineA, mineC, otherX, mineB]);

  // 5. Quick-done checkbox moves the card to "Hoàn thành"; "Hoàn tác" restores it.
  await quickDone(card(todo, quickDoneTitle)).click();
  await expect(card(done, quickDoneTitle)).toBeVisible();
  await expect(todo.getByText(quickDoneTitle)).toHaveCount(0);
  await page.getByRole("button", { name: "Hoàn tác" }).click();
  await expect(card(todo, quickDoneTitle)).toBeVisible();
  await expect(done.getByText(quickDoneTitle)).toHaveCount(0);

  // 6. Collapsing a column writes `collapsed=` to the URL and survives a reload.
  await page.getByRole("button", { name: "Thu gọn cột Đang làm" }).click();
  await expect(page).toHaveURL(new RegExp(`[?&]collapsed=${doingId}(&|$)`));
  await expect(column(page, "Đang làm")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Mở rộng cột Đang làm" })).toBeVisible();
  await page.reload();
  await expect(page).toHaveURL(new RegExp(`[?&]collapsed=${doingId}(&|$)`));
  await expect(page.getByRole("button", { name: "Mở rộng cột Đang làm" })).toBeVisible();
  await page.getByRole("button", { name: "Mở rộng cột Đang làm" }).click();
  await expect(column(page, "Đang làm")).toBeVisible();

  // 7. Column color change in "Cấu hình" re-tints the column body.
  const todoOuter = todo.locator("xpath=..");
  await expect(todoOuter).not.toHaveClass(/bg-sun-100/);
  await page.getByRole("button", { name: "Cấu hình cột" }).click();
  const settings = page.getByRole("dialog", { name: "Cấu hình cột bảng công việc" });
  await settings
    .getByRole("radiogroup", { name: "Màu cột Cần làm" })
    .getByRole("radio", { name: "Vàng nắng" })
    .click();
  await page.keyboard.press("Escape");
  await expect(settings).toBeHidden();
  await expect(todoOuter).toHaveClass(/bg-sun-100/);
  await expect(todoOuter).not.toHaveClass(/bg-cream-100/);

  // 8. A member without `tasks.view_all` sees no quick-filter group and a
  // Board-A-style subtitle instead of the center-wide filter bar.
  const memberContext = await browser.newContext();
  const memberPage = await memberContext.newPage();
  try {
    await loginAsMember(memberPage);
    await memberPage.goto("/tasks");
    await expect(memberPage.getByRole("heading", { name: "Công việc" })).toBeVisible();
    await expect(memberPage.getByRole("group", { name: "Bộ lọc nhanh" })).toHaveCount(0);
    await expect(memberPage.getByText(/^\d+ việc/)).toBeVisible();
    await expect(memberPage.getByText("Toàn trung tâm")).toHaveCount(0);
  } finally {
    await memberContext.close();
  }

  // Clean up the rest of this spec's seed data: `tasks-board-dnd.spec.ts`
  // runs right after this file and drags cards by on-screen pixel position,
  // so leaving 10 extra cards piled into "Cần làm"/"Đang làm" (this spec's
  // own leftovers, beyond `myDone` already deleted above) would shift the
  // column layout it depends on. Restores the board to what this spec found.
  for (const seeded of [...todoSeeds, ...doingSeeds]) {
    await deleteTask(request, token, seeded.id);
  }
});
