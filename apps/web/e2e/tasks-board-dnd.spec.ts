import { expect, test } from "@playwright/test";

import { apiLoginAsOwner, loginAsOwner } from "./helpers/auth.js";
import { card, column, createTasks, findColumnId, topCardTitles } from "./helpers/board.js";
import { dragTo } from "./helpers/drag.js";

test("drag and drop reorders within a column and moves across columns, and the order survives a reload", async ({
  page,
}) => {
  // Suffix keeps this run's titles unique on a reused database (specs run
  // sequentially against one seeded stack).
  const suffix = Date.now();
  const titles = [1, 2, 3].map((n) => `E2E Kéo ${n} ${suffix}`);
  const [first, second, third] = titles;

  await loginAsOwner(page);

  // Seed through the API: every new task lands at the top of its column, so
  // creating 3, 2, 1 shows [1, 2, 3] top to bottom.
  const token = await apiLoginAsOwner(page.request);
  const todoId = await findColumnId(page.request, token, "Cần làm");
  await createTasks(page.request, token, todoId, [...titles].reverse());

  await page.goto("/tasks");
  const todo = column(page, "Cần làm");
  const done = column(page, "Hoàn thành");
  await expect(card(todo, third)).toBeVisible();
  expect(await topCardTitles(todo, 3)).toEqual(titles);

  // 1. Dragging the third card up onto the second one lands it just above it.
  await dragTo(page, card(todo, third), card(todo, second));
  await expect.poll(() => topCardTitles(todo, 3)).toEqual([first, third, second]);
  await page.reload();
  await expect(card(todo, third)).toBeVisible();
  await expect.poll(() => topCardTitles(todo, 3)).toEqual([first, third, second]);

  // 2. Dropping the first card onto another column moves it there (a drop on
  // the column itself appends; on a card, it slots in before that card).
  // The source column keeps its order.
  await dragTo(page, card(todo, first), done, { at: "top" });
  await expect(card(done, first)).toBeVisible();
  await expect.poll(() => topCardTitles(todo, 2)).toEqual([third, second]);
  await page.reload();
  await expect(card(done, first)).toBeVisible();
  await expect.poll(() => topCardTitles(todo, 2)).toEqual([third, second]);
  // The seeder creates no tasks, so this spec owns the top of "Hoàn thành".
  await expect.poll(() => topCardTitles(done, 1)).toEqual([first]);

  // 3. The dropped card is still the headless lib's option: keyboard moves
  // and the roving tabindex keep working after a pointer drag.
  await expect(card(done, first)).toHaveAttribute("role", "option");
  await expect(card(done, first)).not.toHaveAttribute("data-dragging", /.*/);
});
