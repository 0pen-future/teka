# Research: Headless Kanban Library Pattern cho src/lib/kanban

Nguồn: TanStack Table docs, W3C WAI-ARIA APG (Listbox/Grid patterns), react-spectrum react-aria kanban example, eslint-plugin-boundaries npm, TanStack Query optimistic-update guide. Xem link cuối mỗi mục.

## 1. Core state model + useKanban hook (hook-only, không compound component)

**Khuyến nghị: hook-only API (`useKanban`), KHÔNG dùng compound component `<Kanban.Root>`.** Lý do: board có nhiều state phái sinh phức tạp (group-by-column, sort, overdue, counts) cần tính toán tập trung qua selectors — compound component buộc share state qua Context, làm khó test pure logic & dễ re-render thừa. TanStack Table cũng đi hướng "core object + adapter hook", không dùng compound components, chính vì lý do này (nguồn: tanstack.com/table/latest/docs/overview — pattern "core-plus-adapter": `@tanstack/table-core` framework-agnostic, framework adapter chỉ wrap).

```ts
// src/lib/kanban/types.ts
export type ColumnId = string & { readonly __brand: 'ColumnId' };
export type CardId = string & { readonly __brand: 'CardId' };

export interface KanbanColumn {
  id: ColumnId;
  name: string;
  order: number; // 0..7, dùng cho reorder
}

export interface KanbanCard {
  id: CardId;
  columnId: ColumnId;
  position: number; // thứ tự trong column
}

export interface KanbanBoard<TCard extends KanbanCard> {
  columns: KanbanColumn[];
  cards: TCard[];
}

// src/lib/kanban/state.ts — pure reducer, không side effect
export type KanbanAction<TCard extends KanbanCard> =
  | { type: 'columns/reorder'; columnId: ColumnId; direction: 'up' | 'down' }
  | { type: 'cards/moved'; cardId: CardId; toColumnId: ColumnId; toPosition: number }
  | { type: 'board/replaced'; board: KanbanBoard<TCard> }; // sync từ server/optimistic rollback

export function kanbanReducer<TCard extends KanbanCard>(
  state: KanbanBoard<TCard>,
  action: KanbanAction<TCard>,
): KanbanBoard<TCard> { /* switch thuần, trả state mới */ }

// selectors — tách khỏi reducer để test độc lập & memoize
export function selectCardsByColumn<TCard extends KanbanCard>(
  board: KanbanBoard<TCard>,
): Map<ColumnId, TCard[]> { /* group + sort theo position */ }

export function selectOverdueCount<TCard extends KanbanCard & { dueAt?: string }>(
  board: KanbanBoard<TCard>,
  now: Date,
): number { /* ... */ }
```

```ts
// src/lib/kanban/use-kanban.ts
export interface UseKanbanOptions<TCard extends KanbanCard> {
  initialBoard: KanbanBoard<TCard>;
  dataSource: KanbanDataSource<TCard>; // xem mục 2
}

export interface UseKanbanResult<TCard extends KanbanCard> {
  board: KanbanBoard<TCard>;
  cardsByColumn: Map<ColumnId, TCard[]>;
  moveCard: (cardId: CardId, toColumnId: ColumnId) => void;
  reorderColumn: (columnId: ColumnId, direction: 'up' | 'down') => void;
  getColumnProps: (columnId: ColumnId) => ColumnAriaProps; // mục 3
  getCardProps: (cardId: CardId) => CardAriaProps;
}

export function useKanban<TCard extends KanbanCard>(
  opts: UseKanbanOptions<TCard>,
): UseKanbanResult<TCard> {
  const [state, dispatch] = useReducer(kanbanReducer, opts.initialBoard);
  // actions gọi dataSource.moveCard(...) rồi dispatch optimistic action; xem mục 2
}
```

Không compound component nhưng vẫn cho phép app tự bố cục JSX tuỳ ý (mobile 1-cột / desktop multi-cột) vì hook chỉ trả state + prop-getters, không ép cấu trúc DOM — đúng tinh thần "headless" (react-aria, Downshift).

## 2. Inversion of control: KanbanDataSource port + Command pattern

Port interface tách hoàn toàn transport (Command pattern — mỗi thao tác là một "intent" object, headless lib không biết gì về HTTP/TanStack Query):

```ts
// src/lib/kanban/data-source.ts
export interface KanbanDataSource<TCard extends KanbanCard> {
  loadBoard(): Promise<KanbanBoard<TCard>>;
  createColumn(input: { name: string }): Promise<KanbanColumn>;
  renameColumn(id: ColumnId, name: string): Promise<KanbanColumn>;
  reorderColumns(order: ColumnId[]): Promise<void>;
  deleteColumn(id: ColumnId, moveCardsTo: ColumnId): Promise<void>;
  createCard(input: Omit<TCard, 'id'>): Promise<TCard>;
  updateCard(id: CardId, patch: Partial<TCard>): Promise<TCard>;
  moveCard(id: CardId, toColumnId: ColumnId, toPosition: number): Promise<TCard>;
  deleteCard(id: CardId): Promise<void>;
}
```

Lib chỉ dùng interface này (Dependency Inversion — SOLID "D"). App implement adapter cụ thể trong `src/features/tasks/kanban-data-source.ts` dùng TanStack Query mutations bên trong, map lỗi API sang `KanbanError`:

```ts
// src/features/tasks/kanban-data-source.ts (thin adapter, NGOÀI src/lib)
export function useTasksKanbanDataSource(): KanbanDataSource<TaskCard> {
  const moveMutation = useMutation({
    mutationFn: (input: MoveCardInput) => apiClient.tasks.move(input),
    onMutate: async (input) => {
      await queryClient.cancelQueries({ queryKey: ['tasks-board'] });
      const previous = queryClient.getQueryData(['tasks-board']);
      queryClient.setQueryData(['tasks-board'], optimisticallyMove(previous, input));
      return { previous };
    },
    onError: (err, _input, ctx) => {
      if (isConflictOrNotFound(err)) {
        queryClient.invalidateQueries({ queryKey: ['tasks-board'] }); // 404/409 → refetch nguồn thật
      } else if (ctx?.previous) {
        queryClient.setQueryData(['tasks-board'], ctx.previous); // lỗi khác → rollback
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['tasks-board'] }),
  });
  return { moveCard: (id, col, pos) => moveMutation.mutateAsync({ id, col, pos }), /* ... */ };
}
```

Quy tắc rollback vs refetch (theo TanStack Query official guide): 404/409 nghĩa là optimistic state sai lệch với server → `invalidateQueries` để lấy sự thật mới; lỗi mạng/5xx tạm thời → rollback về snapshot cũ rồi cho user thử lại. Nguồn: tanstack.com/query/latest/docs/framework/react/guides/optimistic-updates.

## 3. Keyboard accessibility (không DnD)

**Pattern khuyến nghị: mỗi Column là `role="listbox"` (hoặc `role="group"` chứa list), mỗi Card là `role="option"`, toàn board là `role="application"`/`toolbar` container** — KHÔNG dùng full `grid` pattern (grid dùng cho bảng ô 2 chiều đồng nhất, không khớp "cột có số card khác nhau, card cao thấp khác nhau"). W3C APG Listbox pattern (w3.org/WAI/ARIA/apg/patterns/listbox) khớp hơn: single-select listbox per column, arrow lên/xuống di chuyển focus trong cột, Home/End nhảy đầu/cuối.

Roving tabindex: chỉ 1 card mỗi column có `tabindex=0` (card đang active), còn lại `tabindex=-1`; Tab nhảy giữa các listbox (columns), Arrow Up/Down di chuyển trong column.

Thao tác không-drag: theo kỹ thuật G219 (W3C, "move up/down button" thay vì kéo thả) — áp dụng tương tự cho kanban: shortcut `[` / `]` gọi `moveCard(focusedCardId, adjacentColumnId)`, đồng thời có "Move to ▾" dropdown menu (`role="menu"`) làm phương án chuột. Sau khi move, dùng `aria-live="polite"` region announce "Đã chuyển thẻ X sang cột Y" — không dời focus, giữ focus tại vị trí card (đã theo card sang cột mới) để không mất ngữ cảnh, đúng khuyến nghị "polite status region announces confirmation without moving focus".

Phân chia trách nhiệm lib vs app:
- **Lib (`src/lib/kanban`)**: `useKanbanKeyboard()` xử lý roving tabindex + arrow/`[`/`]` key handling thuần (không CSS/style); prop-getters `getColumnProps(columnId): { role, 'aria-label', onKeyDown }` và `getCardProps(cardId): { role, tabIndex, 'aria-selected', onKeyDown }` trả object thuần để app spread vào JSX (pattern "prop getters" của Downshift/TanStack Table — cho phép app compose thêm handler riêng mà không đụng logic a11y).
- **App (`src/features/tasks`)**: live-region text cụ thể (nội dung tiếng Việt/locale), Hv* dropdown UI cho "Move to", toast/announce styling.

Lưu ý: không fetch được source code cụ thể của react-spectrum react-aria kanban example (react-spectrum.adobe.com/beta/react-aria/examples/kanban.html) trong lần thử — trang chỉ trả metadata, không lấy được chi tiết ARIA roles thực tế. Khuyến nghị đọc trực tiếp source đó trước khi implement để đối chiếu (react-aria team có pattern "hidden action button mở menu chọn đích" cho keyboard alternative của DnD, rất giống "Move to dropdown" plan hiện tại, nhưng cần xác nhận lại bằng code thật, không chỉ suy đoán).

## 4. ESLint boundary enforcement

So sánh (theo npmjs.com/package/eslint-plugin-boundaries + timdeschryver.dev/bits/enforce-module-boundaries-with-no-restricted-imports):

| Tiêu chí | `no-restricted-imports` (patterns) | `eslint-plugin-boundaries` |
|---|---|---|
| Setup | built-in ESLint rule, không cần dep mới | thêm devDependency |
| Độ chính xác | match theo glob path, phải liệt kê thủ công từng zone cấm | khai báo "element types" (`lib`, `feature`, `component`) 1 lần, tự suy luận cặp nào bị cấm |
| Bảo trì khi thêm feature mới | phải sửa rule mỗi lần thêm boundary | tự động áp dụng nếu feature mới khớp pattern element type |
| Độ phức tạp cấu hình | thấp | trung bình-cao |

**Khuyến nghị: dùng `no-restricted-imports` trước** vì chỉ có 1 boundary cần enforce (`src/lib/kanban` không được import `src/features/**`, `src/components/**`, chỉ được import `react`) — dùng plugin chuyên dụng cho 1 rule là over-engineering (vi phạm KISS). Nếu sau này có nhiều boundary hơn (nhiều lib, nhiều layer), nâng cấp `eslint-plugin-boundaries`.

```js
// eslint.config.js (flat config) — thêm override cho src/lib/kanban
{
  files: ['src/lib/kanban/**/*.{ts,tsx}'],
  rules: {
    'no-restricted-imports': ['error', {
      patterns: [
        { group: ['@/features/*', '../../features/*', '**/features/**'], message: 'src/lib/kanban phải transport/UI-agnostic, không import features' },
        { group: ['@/components/*', '**/components/**'], message: 'src/lib/kanban không phụ thuộc design system' },
      ],
    }],
  },
},
```

Test: `vitest` cho reducer/selectors thuần (input/output, không render) đặt tại `src/lib/kanban/*.test.ts`; RTL (`@testing-library/react`) test cho adapter `src/features/tasks/kanban-data-source.test.tsx` — mock `apiClient`, assert optimistic update + rollback khi mutation reject.

## 5. Type-safety patterns

- **Discriminated union actions**: đã minh hoạ ở mục 1 (`KanbanAction` với field `type` literal) — exhaustive switch trong reducer, TypeScript báo lỗi khi thiếu case.
- **Branded IDs**: `ColumnId`/`CardId` là branded string (mục 1) — ngăn truyền nhầm `cardId` vào chỗ cần `columnId` dù cả hai đều `string` lúc runtime.
- **Result-style error mapping** (lib không biết HTTP, chỉ biết domain error):

```ts
// src/lib/kanban/errors.ts
export type KanbanError =
  | { kind: 'not-found'; entity: 'column' | 'card' }
  | { kind: 'conflict' } // stale move, đã bị người khác đổi
  | { kind: 'unknown'; cause: unknown };

export type KanbanResult<T> = { ok: true; value: T } | { ok: false; error: KanbanError };
```

```ts
// src/features/tasks/map-api-error.ts — adapter layer map ApiError -> KanbanError
export function toKanbanError(err: ApiError): KanbanError {
  if (err.status === 404) return { kind: 'not-found', entity: 'card' };
  if (err.status === 409) return { kind: 'conflict' };
  return { kind: 'unknown', cause: err };
}
```

Generic card payload: `TCard extends KanbanCard` (`{ id: CardId; columnId: ColumnId; position: number }`) đã đủ ràng buộc cho core logic (group/sort/move); app tự extend thêm `priority/dueAt/assigneeId` khi khai báo `TaskCard extends KanbanCard`.

## 6. Responsive: 1 state, 2 presenter (Strategy/Presenter pattern)

`useKanban` chỉ trả state + actions, không quyết định layout. App viết 2 presenter dùng chung 1 hook instance:

```tsx
// src/features/tasks/board-desktop.tsx
function BoardDesktop({ kanban }: { kanban: UseKanbanResult<TaskCard> }) {
  return kanban.board.columns.map((col) => <ColumnView key={col.id} {...kanban.getColumnProps(col.id)} />);
}
// src/features/tasks/board-mobile.tsx
function BoardMobile({ kanban }: { kanban: UseKanbanResult<TaskCard> }) {
  const [activeColumnId, setActiveColumnId] = useState(kanban.board.columns[0]?.id);
  return <SegmentedControl ... />; // render 1 ColumnView duy nhất theo activeColumnId
}
```

`useMediaQuery`/breakpoint check ở app layer chọn presenter nào render — không nhân bản logic move/reorder vì cả 2 gọi chung `kanban.moveCard`/`kanban.reorderColumn`.

## 7. Design patterns dùng & trade-off

| Pattern | Áp dụng | Trade-off | Test note |
|---|---|---|---|
| Headless/Hooks | `useKanban`, `useKanbanKeyboard` | tách UI khỏi logic, nhưng app phải tự lo render + a11y wiring đúng prop-getters | test hook bằng `@testing-library/react-hooks` hoặc gọi reducer trực tiếp |
| Adapter | `KanbanDataSource` impl trong features/tasks | cô lập transport, nhưng thêm 1 lớp gián tiếp (mapping) | mock interface dễ, không cần mock HTTP |
| Command | mỗi action (`moveCard`, `reorderColumn`) là intent rời rạc qua dataSource | dễ audit/replay, nhưng nhiều method hơn 1 API "update chung chung" | test từng command độc lập |
| Reducer/Observer | `useReducer` + selectors | state dự đoán được, debug dễ (action log), nhưng cần selector memoize để tránh re-render thừa | test thuần input→output, không cần render |
| Strategy/Presenter | `BoardDesktop`/`BoardMobile` cùng dùng 1 `useKanban` | tránh duplicate logic, nhưng phải giữ presenter "dumb" tuyệt đối | test riêng từng presenter với cùng fixture state |
| Prop Getters | `getColumnProps`/`getCardProps` | app compose thêm handler tự do (giống Downshift), nhưng API "ẩn" hơn so với prop trực tiếp, cần doc rõ | test bằng cách gọi getter rồi assert object trả về |
| Compound Components | **không dùng** cho core, có thể cân nhắc cho phần render optional (vd `<Kanban.LiveRegion>`) nếu cần | quyết định giữ core hook-only để tối đa control cho app + dễ test | n/a |

## Câu hỏi chưa giải quyết
- Chưa lấy được source code thật của react-aria kanban example (fetch chỉ ra metadata) — cần đọc trực tiếp trước khi chốt chi tiết ARIA roles/keyboard shortcut để tránh sai lệch so với pattern đã kiểm chứng của Adobe.
- Chưa rõ team có sẵn `apiClient`/`ApiError` type shape thế nào (scout-api đang làm song song) — mapping ở mục 5 cần khớp lại sau khi có report đó.

Status: DONE_WITH_CONCERNS
Summary: Đề xuất `src/lib/kanban` hook-only (không compound component) theo pattern TanStack Table, với `KanbanDataSource` port (Command pattern) để adapter TanStack Query tự lo optimistic/rollback; a11y dùng listbox-per-column + roving tabindex + `[`/`]` shortcut thay vì grid pattern; ESLint boundary dùng `no-restricted-imports` (đủ cho 1 boundary, tránh over-engineering). Concern: không verify được source code react-aria kanban example thật (fetch fail), nên phần a11y chi tiết cần đối chiếu lại trước khi implement.
