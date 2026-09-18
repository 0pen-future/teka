# `src/lib/kanban` — headless kanban board core

Headless React core for a kanban board: branded types, a pure reducer,
selectors, a `KanbanDataSource` port, and two hooks (`useKanban`,
`useKanbanKeyboard`). No JSX, no CSS, no design system, no TanStack, no
`@/lib/api`. The only dependency is `react`.

## Types

`KanbanBoard<TTask extends KanbanTask, TColumn extends KanbanColumn = KanbanColumn>`
carries a second type parameter for the column shape, defaulted to the lib's
own `KanbanColumn` so every existing `KanbanBoard<TTask>` call site keeps
compiling unchanged. An app that needs extra column fields (Teka: a `color`
swatch the lib itself never reads) declares its own `AppColumn extends
KanbanColumn { color: ... }` and threads it through:

```ts
interface AppColumn extends KanbanColumn {
  color: "none" | "sky" | "sun" | "mint";
}

useKanban<AppTask, AppColumn>({ board, dataSource, messages });
```

`KanbanAction`, `kanbanReducer`, `UseKanbanOptions`, `UseKanbanResult`, and
the three selectors (`selectTasksByColumn`, `selectColumnCounts`,
`selectAdjacentColumnId`) all forward the same `TColumn` parameter, so a
`columns/upserted` action and the board it produces both keep the app's
richer column type end to end — `board.columns[i].color` reads back typed,
with no cast at the app boundary. `KanbanDataSource<TTask>` and
`useKanbanKeyboard` stay untouched: neither one branches on column shape, and
TypeScript's structural (covariant) array typing lets a `KanbanBoard<TTask,
AppColumn>` flow into code still typed for the default `KanbanBoard<TTask>`
without an explicit conversion.

## Accessibility contract (verified before implementation)

Sources read before any prop-getter code was written:

- W3C WAI-ARIA Authoring Practices Guide — Listbox pattern:
  https://www.w3.org/WAI/ARIA/apg/patterns/listbox/
- W3C WAI-ARIA Authoring Practices Guide — Grid pattern (checked and
  rejected — see below): https://www.w3.org/WAI/ARIA/apg/patterns/grid/
- react-aria / React Spectrum drag-and-drop example (checked for a kanban
  reference implementation): the library's DnD hooks target full pointer +
  keyboard drag emulation (pick up with Enter, move focus with Tab/Arrow,
  drop with Enter, cancel with Escape) on top of `role="button"` draggable
  items, not a `grid`/`listbox` semantic split. That full DnD-emulation
  keyboard mode is out of scope for this version (see "Non-goal" below), so it
  was not adopted verbatim; only its bracket-key intent (move an item without
  a live drag gesture) informed the `[`/`]` design already proposed in the
  plan.

**Conclusion — the plan's original proposal held up, with one addition:**

- Each column is `role="listbox"` with an `aria-label` (its name).
- Each task is `role="option"` inside its column's listbox, with
  `aria-selected` following the roving-tabindex focus (single-select
  listbox, "selection follows focus" — the model APG describes for listbox,
  not the grid pattern's separate row/cell navigation model, which does not
  fit uneven-height, single-column-of-cards groupings like kanban columns).
- Roving tabindex is scoped **per column**, not board-wide: exactly one task
  per column has `tabIndex={0}` (the column's current selection), every other
  task in that column has `tabIndex={-1}`. Tab moves focus between columns;
  Arrow Up/Down move focus within the focused column's listbox.
- `Home` / `End` jump to the first/last task in the focused column.
- `[` and `]` move the focused task to the **top** of the previous/next
  column (`selectAdjacentColumnId`, then
  `dataSource.moveTask(taskId, columnId, 0)`) — a no-op at the board's
  edges. This is a deliberately
  simpler alternative to keyboard-emulated drag-and-drop, analogous to the
  WCAG "move up/down button" reordering technique (G219): a discrete,
  non-drag action rather than a drag gesture emulated via focus. "Top" is
  where a move with no explicit neighbour lands server-side in Teka, so the
  announcement, the optimistic update and the refetched board all agree.
- The hook never renders a live region itself. `announcement` (a short
  string set after every keyboard-triggered outcome, e.g. `"Moved task to
Doing."` or `"Could not move task."` by default) is returned from the
  hook; the app is responsible for placing it into its own
  `aria-live="polite"` region. The lib does not localize this string by
  default — see "Locale" below.

**Pointer drag-and-drop is opt-in and lives in the app, not here.** The lib
ships no drag layer and no dnd dependency; it only provides the pure
position helpers below (`resolveDrop` & co.), which turn "dropped at index
i" into the data-source call and the optimistic `position`. Teka's dnd-kit
layer on top of them is the app's own `use-board-dnd` hook under
`src/features/tasks/hooks/`. Whatever the
app adds, `[`/`]` moves stay as they are: they remain the accessible,
low-friction path for keyboard-only and switch-device users, and are not to
be replaced by keyboard-emulated dragging.

## Locale

The lib has no i18n dependency and does not know the app's locale.
`useKanbanKeyboard` (and `useKanban`, which forwards the option) accepts an
optional `messages: UseKanbanKeyboardMessages` — `{ moved: (columnName:
string) => string; moveFailed: string }` — used to build the `announcement`
value after a `[`/`]` move. Omitting it falls back to plain English (`"Moved
task to {column}."` / `"Could not move task."`). A consuming feature that
needs localized announcements passes its own `messages`, e.g. in Teka:

```ts
useKanban({
  board,
  dataSource,
  messages: {
    moved: (columnName) => `Đã chuyển việc sang cột ${columnName}.`,
    moveFailed: "Không chuyển được việc.",
  },
});
```

## Board is a prop, not hook state

Neither `useKanban` nor `useKanbanKeyboard` puts `board` in `useState` or
`useReducer`. `board` is provided by the app (in Teka: a TanStack Query
cache) and returned back from `useKanban` unchanged. The two hooks only own
ephemeral UI state — roving-tabindex focus and the latest `announcement`.

`kanbanReducer` (in `state.ts`) is exported as a **pure utility function**,
not used internally by any hook. The app calls it itself to compute
optimistic cache updates, e.g.:

```ts
queryClient.setQueryData(kanbanKey, (board) =>
  board ? kanbanReducer(board, { type: "tasks/moved", taskId, columnId, position }) : board,
);
```

Do not add a `useReducer(kanbanReducer, board)` inside a hook here. That
would create two sources of truth (the reducer's internal state and the
app's query cache), which diverge silently on refetch. If a future author
is tempted to do this for convenience, don't — see `use-kanban.test.tsx`
for the test that proves board is never copied internally today.

## Ports — `KanbanDataSource<TTask>`

The app implements this interface and passes it into `useKanban` (in Teka:
`src/features/tasks/hooks/use-tasks-data-source.ts`).
The lib only ever calls these 9 methods; it knows nothing about HTTP,
caching, retries, or optimistic updates.

| Method           | Signature                                                        | Intent                                 |
| ---------------- | ---------------------------------------------------------------- | -------------------------------------- |
| `loadBoard`      | `() => Promise<KanbanBoard<TTask>>`                              | Initial/refetch load                   |
| `createColumn`   | `(input: { name, isDone }) => Promise<KanbanColumn>`             | Add column                             |
| `updateColumn`   | `(columnId, patch: { name?, isDone? }) => Promise<KanbanColumn>` | Rename / toggle done-column            |
| `reorderColumns` | `(order: ColumnId[]) => Promise<void>`                           | Persist new column order               |
| `deleteColumn`   | `(columnId, moveTasksTo: ColumnId) => Promise<void>`             | Delete, relocating its tasks           |
| `createTask`     | `(input: Omit<TTask, "id">) => Promise<TTask>`                   | Add task                               |
| `updateTask`     | `(taskId, patch: Partial<Omit<TTask, "id">>) => Promise<TTask>`  | Edit task fields                       |
| `moveTask`       | `(taskId, columnId, position) => Promise<TTask>`                 | Move/reorder a task (drag, or `[`/`]`) |
| `deleteTask`     | `(taskId) => Promise<void>`                                      | Delete task                            |

`moveTask`'s `position` is the **target index** in `columnId`, counted with
the moved task removed from that column: `0` is the top (what `[`/`]` send),
the remaining tasks' length is the bottom. It is not the stored `position`
number. The adapter translates it into the backend's contract — Teka's API
wants "insert after task X", which `afterTaskIdAt` derives from the same
index (see "Position helpers").

Every method resolves to a value or rejects. Rejections should be (or be
mapped to, by the adapter) a `KanbanError` — see `errors.ts` for the union
and the `isKanbanError` type guard. The lib never inspects HTTP status codes
itself; that translation is the adapter's job.

`KanbanDataSource` uses function-property syntax
(`loadBoard: () => Promise<...>`), not method shorthand
(`loadBoard(): Promise<...>`), so that implementations and test fakes stay
plain objects of closures with no implicit `this` binding, and so that bare
references (`expect(dataSource.moveTask).toHaveBeenCalledWith(...)`) don't
trip `@typescript-eslint/unbound-method`.

### Example adapter (illustrative — not exported by this lib)

```ts
// e.g. src/features/<feature>/hooks/use-kanban-data-source.ts
import { apiClient } from "@/lib/api/client";
import { afterTaskIdAt, asColumnId, asTaskId, type KanbanDataSource } from "@/lib/kanban";

export function createKanbanDataSource(boardId: string): KanbanDataSource<TaskCardDto> {
  return {
    loadBoard: async () => {
      const dto = await apiClient.get(`/boards/${boardId}`);
      return {
        columns: dto.columns.map((c) => ({ ...c, id: asColumnId(c.id) })),
        tasks: dto.tasks.map((t) => ({
          ...t,
          id: asTaskId(t.id),
          columnId: asColumnId(t.columnId),
        })),
      };
    },
    moveTask: async (taskId, columnId, position) => {
      // `position` is a target index; this backend wants "after which task".
      const afterTaskId = afterTaskIdAt(tasksInColumn(columnId), position, taskId);
      const dto = await apiClient.patch(`/tasks/${taskId}/move`, { columnId, afterTaskId });
      return { ...dto, id: asTaskId(dto.id), columnId: asColumnId(dto.columnId) };
    },
    // ...remaining 7 methods follow the same shape: call apiClient, cast IDs
    // at the boundary with asColumnId/asTaskId, translate HTTP failures into
    // KanbanError before rejecting.
  };
}
```

## Position helpers

`positions.ts` holds the pure functions an app-level drag layer needs to talk
to the ports above. None of them touch React or the drag library; the app's
`onDragEnd` handler is the only caller.

| Function                                           | Returns              | Use                                                                                            |
| -------------------------------------------------- | -------------------- | ---------------------------------------------------------------------------------------------- |
| `resolveDrop(board, { taskId, overId, overType })` | `DropTarget \| null` | Turn "dropped over task/column X" into `{ columnId, index, afterTaskId }`; `null` = do nothing |
| `afterTaskIdAt(tasks, index, movingId)`            | `TaskId \| null`     | Id of the task above slot `index` (`null` = top)                                               |
| `positionBetween(prev, next)`                      | `number`             | A sortable `position` between two neighbours                                                   |
| `optimisticPositionFor(tasks, index, movingId)`    | `number`             | `positionBetween` applied to the neighbours at `index`                                         |

Two invariants hold everywhere: **`index` is counted on the target column
with the moving task removed**, and **`afterTaskId === null` means top of
the column**. `resolveDrop` follows what a vertical sortable list displays
while dragging, i.e. `arrayMove(items, activeIndex, overIndex)`: over a task
in the same column, dragging down lands the card just _below_ it and
dragging up just _above_ it; over a task in another column, the card is
inserted _before_ it; over a column, the card goes to the bottom. Dropping a
task on itself, on the slot it already occupies (the bottom task of a column
dropped on that column), or over an id that is not on the board, yields
`null` — the caller then skips both the optimistic update and the request.

`positionBetween` may return negative or non-integer values; `selectTasksByColumn`
sorts numerically, so that is fine for an optimistic `tasks/moved`, and the
server is expected to renormalize the stored values on its side.

```ts
function onDragEnd({ active, over }) {
  if (!over) return;
  const taskId = asTaskId(String(active.id));
  const target = resolveDrop(board, {
    taskId,
    overId: String(over.id),
    overType: over.data.current?.type === "column" ? "column" : "task",
  });
  if (!target) return;
  const columnTasks = selectTasksByColumn(board).get(target.columnId) ?? [];
  const position = optimisticPositionFor(columnTasks, target.index, taskId);
  // optimistic: kanbanReducer(board, { type: "tasks/moved", taskId, columnId: target.columnId, position })
  // persist:    dataSource.moveTask(taskId, target.columnId, target.index)
}
```

## Prop getters

`getColumnProps(columnId, extra?)` returns `{ role: "listbox", "aria-label",
onKeyDown }`. `getTaskProps(taskId, extra?)` returns `{ role: "option",
tabIndex, "aria-selected", ref, onKeyDown? }`. Both accept an `extra` object
as the second argument so the app can merge its own handlers
(`getTaskProps(id, { onClick: handleOpenTask })`); the app's `onKeyDown` (if
provided) always runs before the lib's own handler, and the lib's handler is
skipped if the app calls `event.preventDefault()`.

Real keyboard-event handling lives on the **column's** `onKeyDown` only
(event delegation via bubbling from the focused task). The task's own
`onKeyDown` in `getTaskProps` is a pure passthrough for the app's `extra`
handler — do not attach separate keyboard logic there, or a naive app that
wires both column and task `onKeyDown` will double-fire moves.

`getTaskProps` also returns a required `ref` callback. Roving tabindex
changing `tabIndex` alone does not move real DOM/screen-reader focus; the
lib registers each task's DOM node via this `ref` and calls `.focus()`
imperatively when the roving selection changes (including after a `[`/`]`
move resolves and the app's `board` prop reflects the task in its new
column). If the app supplies its own `ref` via `extra`, both refs are
called — pass `extra.ref` through if the app also needs the DOM node.

## Boundary rules

`eslint.config.js` restricts imports for `src/lib/kanban/**/*.{ts,tsx}`:
`@/features/*`, `@/components/*`, `@/lib/api` / `@/lib/api/*`,
`@tanstack/*`, `zod`, and `axios` are all disallowed, each with a message
explaining why. This is enforced by ESLint, not by convention — the lib is
meant to compile and test standalone once extracted.

## Extracting to a separate package

1. Copy `src/lib/kanban/` (minus `__tests__`'s repo-specific test-runner
   config, if any) into a new package directory with its own
   `package.json` declaring `react` as the only `peerDependency`.
2. Move `index.ts`'s exports to the new package's entry point; keep the
   export list unchanged — it is the intended public surface.
3. Point the app's `@/lib/kanban` imports at the new package name (a single
   alias/path-mapping change) instead of moving call sites.
4. Re-run this directory's test suite unmodified inside the new package —
   it has no dependency beyond `react` and `@testing-library/react`, so it
   should pass without edits.
5. Delete the ESLint override block for `src/lib/kanban/**` from
   `apps/web/eslint.config.js` once the directory no longer exists in this
   repo.
