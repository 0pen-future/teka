import { expect, type APIRequestContext, type Locator, type Page } from "@playwright/test";

export function column(page: Page, name: string) {
  return page.getByRole("listbox", { name });
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

/** Creates tasks in order; each new task lands at the top, so the board shows them reversed. */
export async function createTasks(
  request: APIRequestContext,
  token: string,
  columnId: string,
  titles: string[],
): Promise<CreatedTask[]> {
  const created: CreatedTask[] = [];
  for (const title of titles) {
    const response = await request.post("/api/v1/tasks", {
      headers: { Authorization: `Bearer ${token}` },
      data: { title, column_id: columnId },
    });
    expect(response.status(), `create "${title}"`).toBe(201);
    const body = (await response.json()) as { data: { id: string } };
    created.push({ id: body.data.id, title });
  }
  return created;
}
