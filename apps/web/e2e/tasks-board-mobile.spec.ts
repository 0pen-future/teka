import { expect, test } from "@playwright/test";

import { apiLoginAsOwner, loginAsOwner } from "./helpers/auth.js";
import { card, column, createTasks, findColumnId, topCardTitles } from "./helpers/board.js";
import { touchDragTo } from "./helpers/drag.js";

/** Enough cards to push the single-column mobile board past a Pixel 7 viewport. */
const CARD_COUNT = 10;

test.describe("mobile board (touch)", () => {
  test.skip(({ hasTouch }) => !hasTouch, "needs the touch-emulated mobile project");

  test("a long press starts a drag that reorders the column; a quick swipe scrolls instead", async ({
    page,
  }) => {
    const suffix = Date.now();
    const titles = Array.from({ length: CARD_COUNT }, (_, i) => `E2E Chạm ${i + 1} ${suffix}`);
    const [first, second, third] = titles;

    await loginAsOwner(page);
    const token = await apiLoginAsOwner(page.request);
    const todoId = await findColumnId(page.request, token, "Cần làm");
    await createTasks(page.request, token, todoId, [...titles].reverse());

    await page.goto("/tasks");
    const todo = column(page, "Cần làm");
    await expect(card(todo, first)).toBeVisible();
    expect(await topCardTitles(todo, CARD_COUNT)).toEqual(titles);

    // A drop must never open the detail dialog the tap handler would.
    const dialogOpened = () => page.getByRole("dialog", { name: "Chi tiết công việc" }).count();

    // 1. Quick swipe from the third card to the first: no hold, so the touch
    // sensor stays idle and the browser scrolls the page.
    const scrollOffset = () =>
      page.evaluate(() => Math.max(window.scrollY, document.documentElement.scrollTop));
    const before = await scrollOffset();
    expect(
      await page.evaluate(() => document.documentElement.scrollHeight > window.innerHeight),
      "the column must overflow the viewport for a swipe to scroll",
    ).toBe(true);
    await touchDragTo(page, card(todo, third), card(todo, first), { holdMs: 0 });
    await expect.poll(scrollOffset).toBeGreaterThan(before);
    // Give a mistaken drag activation time to render before reading the order.
    await page.waitForTimeout(100);
    expect(await topCardTitles(todo, CARD_COUNT)).toEqual(titles);
    expect(await dialogOpened()).toBe(0);

    // 2. Long press, then move onto the second card: the drag activates and
    // the third card lands just above the second.
    await page.evaluate(() => window.scrollTo(0, 0));
    await touchDragTo(page, card(todo, third), card(todo, second), { holdMs: 300 });
    await expect.poll(() => topCardTitles(todo, 3)).toEqual([first, third, second]);
    expect(await dialogOpened()).toBe(0);
    await page.reload();
    await expect(card(todo, third)).toBeVisible();
    await expect.poll(() => topCardTitles(todo, 3)).toEqual([first, third, second]);
  });
});
