# Research: thư viện drag-and-drop cho kanban board (2026-09-17)

Phạm vi: chọn lib DnD pointer/touch để thêm vào `apps/web/src/features/tasks` như một
interaction mode BỔ SUNG cho core `apps/web/src/lib/kanban` (đã có roving-tabindex +
`[`/`]` keyboard move, không đổi). Yêu cầu: reorder trong cột + move giữa cột (desktop,
tất cả cột hiện side-by-side, scroll ngang), reorder trong 1 cột duy nhất đang hiển thị
(mobile).

## 1. dnd-kit — hiện trạng 2 nhánh song song

Repo `clauderic/dnd-kit` rất active (17.6k star, commit gần nhất 2026-09-12, không
archived — https://github.com/clauderic/dnd-kit), nhưng có **hai API line khác version
scheme, khác mức ổn định**:

| Package | Latest | Publish | peerDeps | Ghi chú |
|---|---|---|---|---|
| `@dnd-kit/core` | 6.3.1 | 2024-12-05 | `react >=16.8.0` | branding hiện tại trên dndkit.com là **"Legacy"** |
| `@dnd-kit/sortable` | 10.0.0 | 2024-12-04 | `react >=16.8.0`, `@dnd-kit/core ^6.3.0` | preset multi-container sortable |
| `@dnd-kit/react` | 0.5.0 | 2026-06-11 | `react ^18 \|\| ^19` | React adapter cho kiến trúc mới |
| `@dnd-kit/dom` | 0.5.0 | 2026-06-11 | (none, framework-agnostic) | engine mới, dựa trên "signal" |

(versions/dates lấy trực tiếp từ `registry.npmjs.org`, không qua tool tóm tắt, nên đáng
tin hơn kết quả search engine.)

Trang chủ https://dndkit.com/ không còn đặt `@dnd-kit/core` làm mặc định — nav chính trỏ
sang `@dnd-kit/react` + `@dnd-kit/dom`, còn `core`/`sortable` bị đẩy vào mục
`/legacy/...`. Tuy vậy **`@dnd-kit/react`/`dom` vẫn ở 0.5.0 (chưa tới 1.0)**, và thảo
luận roadmap chính thức https://github.com/clauderic/dnd-kit/discussions/1842 (câu hỏi
đặt 2025-11-26: "core có bị deprecate không, dự án mới nên dùng gì") **đến nay vẫn 0
phản hồi từ maintainer** — tức chưa có tuyên bố chính thức nào về production-readiness
hay timeline lên v1.

Quan trọng hơn cho use case này: nhánh mới **đã bỏ `TouchSensor` riêng**, gộp chung vào
một `PointerSensor` xử lý cả mouse/touch/pen qua Pointer Events API
(https://dndkit.com/react/guides/sensors/). Cộng đồng đang report hồi quy khi dùng trên
màn cảm ứng so với `TouchSensor` cũ (issue
https://github.com/clauderic/dnd-kit/issues/1723 — "experimental TouchSensor
alternative"). Vì mobile touch-reorder là yêu cầu bắt buộc của project, đây là rủi ro
thật, không phải lý thuyết.

**Kết luận cho câu hỏi 1:** `@dnd-kit/core` 6.x + `@dnd-kit/sortable` vẫn là nhánh ổn
định, đã 1.0+, được document nhiều nhất cho pattern kanban, và peerDep lỏng
(`>=16.8.0`) nên tương thích React 19.2 của project mà không cần override. Nhánh
`@dnd-kit/react`/`dom` là hướng tương lai nhưng còn pre-1.0 và có regression đã biết ở
đúng phần touch mà project cần — không nên chọn cho một tính năng production ngay bây
giờ.

## 2. Atlassian `@atlaskit/pragmatic-drag-and-drop`

| Package | Latest | Publish |
|---|---|---|
| `@atlaskit/pragmatic-drag-and-drop` | 3.1.0 | 2026-08-29 |
| `-hitbox` | 2.2.2 | 2026-09-16 |
| `-auto-scroll` | 3.2.0 | 2026-08-29 |
| `-live-region` | 2.1.0 | 2026-08-29 |

Repo cực active (12.8k star, push 2026-09-17, https://github.com/atlassian/pragmatic-drag-and-drop),
không có `peerDependencies` khai báo vì lib framework-agnostic thuần (chỉ thao tác qua
`draggable()`/`dropTargetForElements()` trên DOM ref) — nên "React 19 status" của nó
không phải vấn đề version, mà là cách gắn vào React qua `useEffect` + ref, hoàn toàn
không phụ thuộc React version.

**Vấn đề touch/mobile — đây là điểm quyết định loại lib này khỏi use case hiện tại**:
lib build trên **native HTML5 Drag and Drop API** của browser (dựa trên mouse events),
không phải Pointer Events. HTML5 DnD API vốn không có touch support chuẩn trên browser
mobile. Bằng chứng cụ thể:

- Thảo luận chính thức "Mobile/Touch Support?"
  (https://github.com/atlassian/pragmatic-drag-and-drop/discussions/93): user báo drag
  fail ~90% trên touch, thời gian press-and-hold quá dài; thread vẫn "Unanswered", có
  người xác nhận lại vấn đề tới 2025-09-11.
- Issue đang mở https://github.com/atlassian/pragmatic-drag-and-drop/issues/204
  ("draggable element isnt working on touchscreen", mở 2025-04-11, chưa đóng).
- Giải pháp phổ biến cho HTML5 DnD trên touch là polyfill ngoài (`drag-drop-touch`,
  `mobile-drag-drop`) dịch touch event thành pointer/mouse event giả — tức bản thân lib
  không tự giải quyết, phải thêm tầng workaround không chính thức.

Bundle size core rất nhỏ (~4.7kB gzip theo blog Atlassian
https://www.atlassian.com/blog/design/designed-for-delight-built-for-performance —
`bundlephobia` trả về core entry chỉ 176B vì đây là "meta" package re-export, size thật
nằm ở các entry point con như `element/adapter`).

**Kết luận:** lib rất tốt cho desktop-only hoặc nơi có thể chấp nhận rủi ro touch (ví
dụ Jira/Confluence — sản phẩm gốc của Atlassian — ưu tiên desktop). Với yêu cầu
"mobile hiển thị 1 cột, reorder bằng touch" của project này, dùng native HTML5 DnD là
rủi ro đã được cộng đồng xác nhận chưa fix — **không recommend**.

## 3. Các lựa chọn khác

- **`@hello-pangea/dnd`** (fork cộng đồng của `react-beautiful-dnd` đã bị Atlassian
  archive): latest `18.0.1`, publish 2025-02-09, `peerDependencies` đã hỗ trợ
  `react ^18 || ^19` — tức tương thích React 19.2 của project
  (https://www.npmjs.com/package/@hello-pangea/dnd). Repo vẫn active maintenance. Tuy
  nhiên bundle nặng hơn hẳn (~28kB gzip, xem mục 7), API dựa trên list-reorder
  (`Droppable`/`Draggable`) không thiết kế sẵn cho multi-column kanban linh hoạt bằng
  dnd-kit (không có `DragOverlay`, cross-container preview phải tự chế), và không có
  gói tối ưu riêng cho touch/mobile ngoài default. Là phương án dự phòng hợp lý nếu sau
  này `@dnd-kit/core` ngừng hẳn maintenance, nhưng không tốt hơn dnd-kit cho use case
  hiện tại.
- **`react-aria` DnD hooks** (`@react-aria/dnd`, cùng nhà `react-aria` latest 3.52.1,
  publish 2026-09-04, peerDep `react ^16.8 || ... || ^19` — rất mới và tương thích
  React 19): API thiết kế quanh accessibility-first (drag emulation qua phím Enter/Tab,
  gần giống mô hình mà README của `apps/web/src/lib/kanban` đã đọc và **chủ động từ
  chối** vì không khớp mô hình `listbox`/`option` hiện có — xem
  `apps/web/src/lib/kanban/README.md` mục "Accessibility contract"). Dùng react-aria
  DnD nghĩa là phải hoà giải 2 mô hình a11y khác nhau trong cùng 1 board — vi phạm
  KISS. Không recommend cho bổ sung lần này.

## 4. Pattern multi-container sortable với dnd-kit (khuyến nghị triển khai)

Nguồn: official docs sensors (`https://dndkit.com/api-documentation/sensors`,
redirect từ `docs.dndkit.com`) + kiến thức API ổn định từ v6 (không đổi từ 2022, dùng
trong ví dụ multi-container chính thức của dnd-kit storybook).

**Sensor — chỉ pointer + touch, bỏ hẳn `KeyboardSensor`:**

```tsx
import { PointerSensor, TouchSensor, useSensor, useSensors } from "@dnd-kit/core";

const sensors = useSensors(
  useSensor(PointerSensor, { activationConstraint: { distance: 8 } }), // tránh drag khi chỉ click
  useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }), // mobile: giữ 250ms
);
// Không liệt kê KeyboardSensor/MouseSensor => DndContext không kích hoạt chúng.
// `[`/`]` keyboard move của lib hiện có tiếp tục hoạt động y nguyên vì nó không
// đi qua DndContext, không có xung đột.
```

`DndContext` **không tự thêm sensor mặc định nào nếu bạn truyền prop `sensors`** — nên
việc "tắt KeyboardSensor" chỉ đơn giản là không đưa nó vào mảng, không cần flag riêng.

**Cross-column live preview (`onDragOver` di chuyển item sang container khác bằng bản
copy tạm thời trong state optimistic, không commit ngay vào TanStack Query cache):**

```tsx
function handleDragOver(event: DragOverEvent) {
  const { active, over } = event;
  if (!over) return;
  const activeColumnId = active.data.current?.columnId;
  const overColumnId = over.data.current?.columnId ?? over.id; // over là task hoặc column rỗng
  if (activeColumnId === overColumnId) return;
  // cập nhật state cục bộ (KHÔNG gọi dataSource.moveTask ở đây) để card
  // "nhảy" cột ngay khi kéo qua, cho cảm giác preview trực quan
  setLocalBoard((board) => kanbanReducer(board, {
    type: "tasks/moved", taskId: active.id, columnId: overColumnId, position: 0,
  }));
}

function handleDragEnd(event: DragEndEvent) {
  // chỉ tại đây mới gọi dataSource.moveTask(...) để persist vị trí cuối cùng,
  // giống hệt cách `[`/`]` gọi moveTask — dùng chung 1 optimistic-update path
  // qua kanbanReducer đã có sẵn trong lib (state.ts), tránh 2 nguồn sự thật.
}
```

`kanbanReducer` (pure, đã export từ `apps/web/src/lib/kanban/state.ts`) tái dùng được
trực tiếp cho cả optimistic update của `onDragOver`/`onDragEnd` lẫn `[`/`]` — không cần
viết logic move riêng cho DnD.

**`DragOverlay`** (render card đang kéo tách khỏi DOM flow, tránh layout jump khi
`transform` của item gốc và overlay lệch nhau):

```tsx
<DragOverlay>{activeTask ? <TaskCard task={activeTask} isOverlay /> : null}</DragOverlay>
```

**Cột rỗng vẫn là drop target hợp lệ** — dùng `useDroppable` bọc ngoài
`SortableContext` của cột (kể cả khi `items=[]`, `useDroppable` vẫn đăng ký `over`
event bình thường, không phụ thuộc số lượng item con):

```tsx
const { setNodeRef } = useDroppable({ id: column.id, data: { columnId: column.id } });
<div ref={setNodeRef}>
  <SortableContext items={taskIds} strategy={verticalListSortingStrategy}>
    {tasks.map((t) => <SortableTaskCard key={t.id} task={t} />)}
  </SortableContext>
</div>
```

**Auto-scroll cho board scroll ngang (desktop, nhiều cột):** `DndContext` có
auto-scroll built-in (`autoScroll` prop, mặc định bật, theo dõi `scrollableAncestors`);
với container scroll ngang tuỳ biến (không phải `window`), cần truyền
`autoScroll={{ layoutShiftCompensation: false }}` hoặc chỉ định rõ container qua
`MeasuringStrategy`/`dndContext` — cấu hình chi tiết nên xác nhận lại bằng test thủ công
trên chính container `overflow-x-auto` của board khi implement, vì hành vi auto-scroll
theo custom scroll container ít được test bởi maintainer hơn `window` scroll.

## 5. Accessibility — xung đột với `role="listbox"`/`option` hiện có

`useDraggable`/`useSortable` của dnd-kit mặc định set lên phần tử được gắn
`listeners`:

| Attribute | Default |
|---|---|
| `role` | `"button"` |
| `tabIndex` | `0` |
| `aria-roledescription` | `"draggable"` |
| `aria-describedby` | trỏ tới instructions ẩn do dnd-kit tự inject vào `document.body` |

Đây là xung đột trực tiếp với hợp đồng a11y hiện có của lib (mỗi cột `role="listbox"`,
mỗi task `role="option"`, chỉ 1 task/cột có `tabIndex=0` theo roving-tabindex — xem
`apps/web/src/lib/kanban/README.md`). Nếu spread `{...attributes} {...listeners}`
thẳng lên card, sẽ có 2 hệ thống tabIndex tranh nhau và role bị ghi đè thành `button`.

**Cách tránh double-focus-management (khuyến nghị):**

1. Spread `attributes`/`listeners` của dnd-kit **trước**, rồi override bằng props của
   `getTaskProps` (`useKanbanKeyboard`) **sau** — thứ tự prop JSX sau cùng thắng:

```tsx
<div
  ref={setSortableNodeRef}
  {...attributes}
  {...listeners}
  {...getTaskProps(task.id, { style })}  // role="option", tabIndex, aria-selected, ref
                                          // ghi đè role="button"/tabIndex/aria-roledescription của dnd-kit
>
```

   `getTaskProps` merge `ref` (đã hỗ trợ sẵn `mergeRefs` trong hook — xem
   `use-kanban-keyboard.ts`), nên `setSortableNodeRef` của dnd-kit và `ref` roving-focus
   của lib cùng tồn tại không xung đột.

2. Dùng `useDraggable`/`useSortable({ attributes: { role: undefined, roleDescription:
   undefined } })` nếu muốn tắt hẳn từ nguồn thay vì override qua JSX — API hỗ trợ
   custom `attributes` object chính thức.
3. `aria-describedby` do dnd-kit tự thêm (trỏ instructions ẩn) không xung đột role, có
   thể giữ nguyên hoặc thay bằng `screenReaderInstructions` custom tiếng Việt.
4. Tắt hẳn `announcements` mặc định của dnd-kit (`live-region` riêng nó tự tạo) và tái
   dùng `aria-live` region đã có sẵn (`announcement` return từ `useKanbanKeyboard`) —
   truyền `announcements` prop rỗng hoặc custom cho `DndContext` để tránh 2 live-region
   cùng đọc 2 message khác nhau cho 1 hành động kéo-thả.

Nguồn: https://dndkit.com/legacy/guides/accessibility/ (default ARIA attributes),
https://docs.dndkit.com/guides/accessibility (announcements/`screenReaderInstructions`
API — redirect nhưng nội dung tương đương legacy doc vì API accessibility không đổi
giữa các bản core 6.x).

## 6. Testing

**Unit test (Vitest + jsdom):** giới hạn đã biết — jsdom không tính layout thật, nên
`getBoundingClientRect()` trả về giá trị 0 cố định cho mọi phần tử; dnd-kit tính
collision/vị trí dựa trên rect này nên **không mô phỏng được một kéo-thả thật sự bằng
`fireEvent`/`user-event` trong jsdom** (xác nhận qua issue chính thức
https://github.com/clauderic/dnd-kit/issues/261 "Testing dndkit using React Testing
Library"). Cách khả thi:

- Mock `HTMLElement.prototype.getBoundingClientRect` (spy trả DOMRect cố định theo từng
  test case) để giả lập vị trí phần tử trước khi bắn `pointerdown`/`pointermove`/
  `pointerup`.
- `testing-library/dom-testing-library` có giới hạn khi tạo `PointerEvent` với
  `clientX`/`clientY` qua `createEvent` (issue
  https://github.com/testing-library/dom-testing-library/issues/558) — nên bắn event
  thô bằng `new PointerEvent(...)` + `element.dispatchEvent(...)` thay vì qua helper
  của testing-library.
- **Khuyến nghị thực dụng:** chỉ unit-test phần logic thuần (reducer/`onDragOver`
  mapping, `kanbanReducer` calls) tách khỏi dnd-kit, KHÔNG cố mô phỏng gesture kéo thật
  trong jsdom — việc đó đẩy sang Playwright.

**Playwright (e2e):** `page.dragTo()` built-in thường **không hoạt động** với dnd-kit vì
lib xử lý bằng Pointer Events tự custom, không phải HTML5 DnD mà Playwright's `dragTo`
giả định (xác nhận qua nhiều report cộng đồng, ví dụ issue
https://github.com/microsoft/playwright/issues/20254). Cách đáng tin cậy: dùng
low-level API thủ công —

```ts
const source = page.getByTestId("task-1");
const target = page.getByTestId("column-doing");
await source.hover();
await page.mouse.down();
// nhiều bước move nhỏ — dnd-kit cần vượt activationConstraint.distance rồi mới
// bắt đầu drag, và cần đủ pointermove event để tính lại collision mỗi lần
await page.mouse.move(targetX, targetY, { steps: 15 });
await page.mouse.up();
```

Vì có `TouchSensor` với `delay: 250`, test mobile-viewport cần
`await page.waitForTimeout(260)` giữa `mouse.down()`/`touchscreen` tương ứng trước khi
move — nếu không, activation constraint bị bỏ qua và Playwright chỉ tạo ra 1 click, dự
kiến cần viết riêng 1 helper `dragCard(page, from, to, { touch: true })` cho e2e.

## 7. Ước lượng bundle size (min+gzip)

| Package | gzip |
|---|---|
| `@dnd-kit/core` 6.3.1 | ~13.9 kB |
| `@dnd-kit/sortable` 10.0.0 | ~3.6 kB |
| `@dnd-kit/utilities` (thường đi kèm) | ~1–2 kB (không lấy được số chính xác qua bundlephobia lần chạy này) |
| **Tổng dnd-kit cho kanban** | **~18–19 kB gzip** |
| `@atlaskit/pragmatic-drag-and-drop` core | ~4.7 kB (theo blog chính chủ; cần cộng thêm `-hitbox`, `-auto-scroll`, `-live-region` cho tính năng tương đương → ước lượng thực tế ~10–15 kB, không tự tính được chính xác vì bundlephobia không resolve các sub-entry) |
| `@hello-pangea/dnd` 18.0.1 | ~28.1 kB |

(Nguồn số đo: `bundlephobia` API gọi trực tiếp lúc nghiên cứu 2026-09-17; entry point
`hitbox`/`auto-scroll` của Atlassian trả lỗi resolve qua API này, phải lấy số công bố
từ blog thay thế.)

## Khuyến nghị

| Lib | Version ghim | Vì sao |
|---|---|---|
| **`@dnd-kit/core`** | `^6.3.1` | Stable 1.0+, peerDep lỏng khớp React 19.2, pattern multi-container kanban là use case tài liệu hoá nhiều nhất |
| **`@dnd-kit/sortable`** | `^10.0.0` | Preset chính thức cho reorder trong 1 container + cross-container, pin cùng dòng major với core |
| **`@dnd-kit/utilities`** | theo `^6.x` tương ứng | `CSS.Transform.toString` helper, tránh tự viết lại |

**Không chọn `@atlaskit/pragmatic-drag-and-drop`** dù mới hơn, nhẹ hơn, maintain tích
cực hơn — vì dựa trên native HTML5 DnD, touch trên mobile được chính cộng đồng xác nhận
chưa hoạt động ổn định (issue mở, discussion chưa trả lời), mà mobile single-column
reorder là yêu cầu bắt buộc, không phải nice-to-have.

**Không chọn `@dnd-kit/react`/`@dnd-kit/dom`** (nhánh mới) vì còn pre-1.0, câu hỏi
roadmap chính thức chưa được trả lời, và đã bỏ `TouchSensor` chuyên biệt — đúng phần
project cần nhất lại là phần kém ổn định nhất của nhánh mới.

Rủi ro chấp nhận: `@dnd-kit/core`/`sortable` không có release mới từ 2024-12 (bị đóng
băng feature, chỉ còn ở nhánh "Legacy" trên trang chủ) — về lâu dài dự án nên theo dõi
`discussions/1842` để biết khi nào `@dnd-kit/react` ổn định và TouchSensor tương đương
quay lại, nhưng cho tính năng cần ship bây giờ, "Legacy" ở đây nghĩa là "đã đông cứng
API ổn định", không phải "sắp gỡ bỏ" — repo chính vẫn active, core vẫn cài đặt và chạy
bình thường trên React 19.

## Câu hỏi chưa giải quyết (Unresolved questions)

1. Hành vi auto-scroll của `DndContext` với container `overflow-x-auto` tuỳ biến (không
   phải `window`) chưa được xác minh bằng test thật trong report này — cần thử nghiệm
   thủ công khi implement, vì tài liệu chính thức chủ yếu minh hoạ scroll ở `window`.
2. Chưa đo được bundle size chính xác cho `@dnd-kit/utilities` và các sub-entry của
   `@atlaskit/pragmatic-drag-and-drop-hitbox`/`-auto-scroll` qua bundlephobia (API lỗi
   resolve) — số cho Atlassian stack chỉ là ước lượng gián tiếp.
3. Chưa test thực tế `TouchSensor` `delay: 250ms` có đủ ngắn để UX mobile "mượt" theo
   kỳ vọng người dùng cuối của Teka hay cần tinh chỉnh — đây là quyết định UX cần
   người dùng/PM xác nhận sau khi có prototype.
4. Timeline `@dnd-kit/react` đạt v1 stable là ẩn số hoàn toàn (0 phản hồi maintainer
   trong thread roadmap) — nên coi là "theo dõi định kỳ", không đặt lịch di trú cụ thể.
