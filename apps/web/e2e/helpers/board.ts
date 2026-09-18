import { expect, type APIRequestContext, type Locator, type Page } from "@playwright/test";

export function column(page: Page, name: string) {
  return page.getByRole("listbox", { name });
}

/** Escapes regex metacharacters so a chip label can be dropped into a `new RegExp`. */
function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * A `BoardFilterBar` chip (status or teacher) by its label. Each chip is an
 * `aria-pressed` button whose accessible name is the label followed by its
 * count (e.g. "Quá hạn 1"); this matches a `^label \d+$` regex rather than
 * the literal text.
 */
export function filterChip(page: Page, name: string) {
  return page.getByRole("button", { name: new RegExp(`^${escapeRegExp(name)} \\d+$`) });
}

/** The quick-done button hanging over a card's leading edge; only rendered while the task is open. */
export function quickDone(taskCard: Locator) {
  return taskCard.getByRole("button", { name: "Đánh dấu hoàn thành" });
}

/**
 * Waits for the board's `GET /tasks/board` response, optionally asserting
 * the `filter`/`assignee` query params it was sent with (D3: filtering is
 * server-side, so the params on the wire are the behavior under test, not
 * just the rendered cards). Set up before the action that triggers the
 * request, then `await` after.
 */
export function waitForBoard(page: Page, match: { filter?: string; assignee?: string } = {}) {
  return page.waitForResponse((response) => {
    const url = response.url();
    if (!url.includes("/tasks/board") || !url.includes("today=")) return false;
    if (match.filter !== undefined && !url.includes(`filter=${match.filter}`)) return false;
    if (match.assignee !== undefined && !url.includes(`assignee=${match.assignee}`)) return false;
    return true;
  });
}

/** The accessible name is a substring match, so the full title with its suffix is enough. */
export function card(scope: Locator | Page, title: string) {
  return scope.getByRole("option", { name: title });
}

/**
 * Titles of the top `count` cards in a column, as the board renders them.
 * Specs share one seeded database and every new task lands at the top, so
 * a spec only ever owns the leading cards of a column; whatever an earlier
 * run left below them is ignored.
 *
 * Relies on the card body rendering the title as its first `<p>`; the
 * description preview is a later `<p>`, so a reordered card body would make
 * every spec compare previews instead.
 */
export async function topCardTitles(list: Locator, count: number): Promise<string[]> {
  const titles = await list
    .getByRole("option")
    .evaluateAll((cards) => cards.map((node) => node.querySelector("p")?.textContent ?? ""));
  return titles.slice(0, count);
}

interface BoardColumn {
  id: string;
  name: string;
}

interface CreatedTask {
  id: string;
  title: string;
}

/** Resolves a column id by name from `GET /tasks/board`; the API is the source of truth. */
export async function findColumnId(request: APIRequestContext, token: string, name: string) {
  const response = await request.get("/api/v1/tasks/board", {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.ok()).toBe(true);
  const body = (await response.json()) as { data: { columns: BoardColumn[] } };
  const match = body.data.columns.find((item) => item.name === name);
  if (!match) throw new Error(`column "${name}" not found on the board`);
  return match.id;
}

/** Resolves a center member's teacher id by display name from the member directory. */
export async function findMemberId(request: APIRequestContext, token: string, displayName: string) {
  const response = await request.get("/api/v1/centers/me/members/directory", {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.ok()).toBe(true);
  const body = (await response.json()) as { data: { teacher_id: string; display_name: string }[] };
  const match = body.data.find((item) => item.display_name === displayName);
  if (!match) throw new Error(`member "${displayName}" not found in the directory`);
  return match.teacher_id;
}

/** Per-task overrides for `createTasks`; a plain string still means "just a title". */
export interface TaskSeed {
  title: string;
  dueOn?: string;
  assigneeId?: string;
}

/** Creates tasks in order; each new task lands at the top, so the board shows them reversed. */
export async function createTasks(
  request: APIRequestContext,
  token: string,
  columnId: string,
  tasks: (string | TaskSeed)[],
): Promise<CreatedTask[]> {
  const created: CreatedTask[] = [];
  for (const item of tasks) {
    const seed: TaskSeed = typeof item === "string" ? { title: item } : item;
    const response = await request.post("/api/v1/tasks", {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        title: seed.title,
        column_id: columnId,
        ...(seed.dueOn !== undefined ? { due_on: seed.dueOn } : {}),
        ...(seed.assigneeId !== undefined ? { assignee_id: seed.assigneeId } : {}),
      },
    });
    expect(response.status(), `create "${seed.title}"`).toBe(201);
    const body = (await response.json()) as { data: { id: string } };
    created.push({ id: body.data.id, title: seed.title });
  }
  return created;
}

/** Moves a task server-side (used to seed board state e2e can't reach through the UI alone, e.g. a pre-done task). */
export async function moveTask(
  request: APIRequestContext,
  token: string,
  taskId: string,
  columnId: string,
  afterTaskId?: string,
) {
  const response = await request.post(`/api/v1/tasks/${taskId}/move`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { column_id: columnId, after_task_id: afterTaskId ?? null },
  });
  expect(response.status(), `move task ${taskId}`).toBe(200);
}

/**
 * Soft-deletes a task server-side. Used to clean up seed data a spec must not
 * leave behind for a later spec sharing the same database — e.g. a task
 * parked in a column another spec assumes it exclusively owns.
 */
export async function deleteTask(request: APIRequestContext, token: string, taskId: string) {
  const response = await request.delete(`/api/v1/tasks/${taskId}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status(), `delete task ${taskId}`).toBe(204);
}
