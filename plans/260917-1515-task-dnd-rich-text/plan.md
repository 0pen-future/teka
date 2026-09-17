---
title: "Kanban kéo-thả + mô tả rich text"
description: "Kéo-thả việc (đổi cột, sắp thứ tự trong cột) trên bảng Kanban và soạn mô tả việc bằng rich text; nối tiếp plan 260913-1102-task-center-kanban, giữ nguyên biên giới lib pkg/kanban + src/lib/kanban"
status: in-progress
priority: P1
effort: "6.5d"
tags: [api, web, kanban, dnd, rich-text, lib]
created: 2026-09-17
blockedBy: []
blocks: []
branch: master
---

# Kanban kéo-thả + mô tả rich text

## Overview

Plan này nối tiếp [260913-1102-task-center-kanban](../260913-1102-task-center-kanban/plan.md)
(đã hoàn thành, deploy ngày 13/09). Hai năng lực mới:

1. **Kéo-thả việc (drag and drop)**: người dùng kéo thẻ việc sang cột khác hoặc
   đổi thứ tự trong cùng cột bằng chuột (desktop) và cảm ứng (mobile, chỉ trong
   cột). Đường "Chuyển" menu và phím `[`/`]` giữ nguyên là đường không kéo-thả
   (README lib đã ghi rõ: DnD nếu thêm phải là chế độ opt-in bổ sung).
2. **Mô tả rich text**: form tạo/sửa việc dùng trình soạn thảo (đậm, nghiêng,
   gạch chân, gạch ngang, danh sách, liên kết) thay cho `<textarea>`. Người
   không phải creator xem mô tả ở dạng read-only đã render.

Ba ràng buộc cấu trúc mà plan phải giải:

- **API move chưa có vị trí**: `POST /tasks/:id/move` v1 chỉ nhận `column_id`,
  server luôn đặt lên đầu cột (`topPositionInColumn` = min−1). Sắp thứ tự trong
  cột cần hợp đồng mới, tương thích ngược.
- **Mô tả là plain text, chưa có renderer**: `description TEXT NOT NULL DEFAULT ''`,
  Gin `max=4000`, web zod `max(2000)`. Cần chốt định dạng lưu trữ, sanitize
  phía server và migration dữ liệu cũ.
- **Biên giới lib**: `src/lib/kanban` chỉ được import `react` (ESLint
  `no-restricted-imports`), `pkg/kanban` chỉ stdlib + `uuid`
  (`import_boundary_test.go`). Thư viện DnD và editor **chỉ** sống ở tầng
  feature; lib chỉ nhận thêm helper thuần.

## Environment

- Monorepo `apps/api` (Go 1.25, Gin, GORM, Postgres, golang-migrate, swag) và
  `apps/web` (Vite 8, React 19.2, TanStack Query 5, react-hook-form 7 + zod 4,
  Tailwind 4, radix-ui, Vitest 4 + jsdom + MSW 2, Playwright).
- Migration kế tiếp: `000023`. Chưa có thư viện DnD, editor, sanitizer nào
  được cài (go.mod chưa có `bluemonday`; package.json chưa có `@dnd-kit/*`,
  `@tiptap/*`, `dompurify`).
- Gates: `make test-api-unit` (kèm `scopelint`), `make test-api` (chạy đơn lẻ,
  tuần tự — xem memory), `make lint-api`, `make api-docs`, `make test-web`,
  `make lint-web`, e2e trên stack cô lập `teka-e2e` (target `make e2e` hiện
  chạy vào stack dev; phase 6 thêm `make e2e-isolated`).
- Người dùng cuối là giáo viên/nhân viên trung tâm, dùng cả điện thoại; mọi
  chuỗi UI tiếng Việt.

## Decisions (đã chốt, không mở lại)

Các quyết định D1–D11 của plan trước (biên giới lib, policy 2 tầng, không CAS,
catalog v4, …) vẫn đóng. Plan này thêm:

| # | Quyết định | Lý do / bằng chứng |
|---|-----------|---------------------|
| D1 | Thư viện DnD: `@dnd-kit/core@6.3.1` + `@dnd-kit/sortable@10.0.0` + `@dnd-kit/utilities`, **chỉ** trong `src/features/tasks`. | Trưởng thành (17.6k sao), peer `react >=16.8` chạy React 19, có `TouchSensor` + `PointerSensor` sẵn, ~18 KB gzip. `@dnd-kit/react` 0.5.0 còn pre‑1.0. `@atlaskit/pragmatic-drag-and-drop` dựa HTML5 DnD nên cảm ứng lỗi (issue #204 mở, press-and-hold dài). `@hello-pangea/dnd` tự áp `role="button"` + nhấc bằng Space, xung đột hợp đồng `listbox`/`option`, 29 KB. |
| D2 | DnD là chế độ **bổ sung**; "Chuyển" menu và `[`/`]` giữ nguyên và vẫn đặt việc **lên đầu cột** như v1. Lib đổi keyboard từ index cuối sang index 0 để nhất quán. | README lib: "add it as an additional, opt-in interaction mode — do not replace `[`/`]`". Giữ hành vi v1 cho đường không kéo-thả tránh đổi UX người dùng đã quen. |
| D3 | Hợp đồng move v2: `POST /tasks/:id/move` body `{ "column_id", "after_task_id"?: uuid \| null }`. Thiếu hoặc `null` = đầu cột (đúng v1). | Tương thích ngược 100%: client cũ không đổi gì. Ý nghĩa rõ ràng hơn `position` số. |
| D4 | Server tự tính `position` (float midpoint giữa `after` và việc kế tiếp; cuối cột = `after + 1`) trong cùng transaction; **renormalize** cả cột về `0..n−1` khi khoảng cách `< 1e-6`. Client không gửi float. | Không tin float từ client; tránh trôi độ chính xác sau nhiều lần chèn (README core "Position strategy" đã cảnh báo). |
| D5 | `pkg/kanban` thêm hàm thuần `positionAfter` + 2 port `ListColumnPositions`, `RenormalizeColumn`; không thêm dependency. | Giữ `import_boundary_test.go` xanh; logic vị trí test được bằng fake repo. |
| D6 | Rich text lưu là **HTML subset đã sanitize** trong cột `description` hiện có; không thêm cột, không cờ định dạng. Allowlist: `p, br, strong, em, u, s, ul, ol, li, a[href]`. | Một nguồn sự thật, không nhân đôi dữ liệu; TipTap xuất/nhập HTML trực tiếp. |
| D7 | Sanitize phía server bằng `github.com/microcosm-cc/bluemonday@v1.0.27` trong `internal/features/tasks` (không vào core). Input không bắt đầu bằng `<` được coi là văn bản thuần và bọc `<p>` + escape như migration trước khi sanitize. Giới hạn: body thô `≤ 20000` rune, **văn bản thuần sau sanitize `≤ 4000` rune**; rỗng sau sanitize (vd `<p></p>`) chuẩn hoá về `""`. `href` chỉ `http/https/mailto` (scheme khác → bluemonday bỏ hẳn `<a>`), thêm `rel="noopener noreferrer nofollow"` + `target="_blank"`. | Server là ranh giới tin cậy; mọi client (kể cả tương lai) đều được bảo vệ; client cũ gửi text có xuống dòng không thành "nửa HTML". |
| D8 | Migration `000023` bọc **mọi** mô tả không rỗng thành `<p>…</p>` đã escape HTML, `\n` → `<br>` (không guard theo nội dung — `schema_migrations` lo idempotency). Bắt buộc **backup DB trước khi chạy** trên prod (compose prod tự chạy migrate trước api → backup trước `compose up`). Down = strip tag best-effort (lossy, ghi rõ). | Không có mô tả nào ở trạng thái "nửa HTML nửa text"; renderer không phải đoán định dạng. |
| D9 | Editor: TipTap 3.31.x (`@tiptap/react`, `@tiptap/starter-kit` — v3 stable, MIT, gồm link/underline/strike/list; `CharacterCount` từ `@tiptap/extensions`), tắt các extension ngoài allowlist; toolbar đọc trạng thái qua `useEditorState`. Nạp **lazy** (`React.lazy`) bên trong modal; thẻ việc và chế độ read-only **không** nạp editor. | Chunk editor (~100 KB gzip ước tính) chỉ tải khi mở form; bảng vẫn nhẹ. |
| D10 | Render read-only qua `RichTextView` = `dangerouslySetInnerHTML` **sau** DOMPurify 3.4.15 với cùng allowlist (defense in depth). | Chi phí ~9 KB gzip; chặn XSS nếu server hoặc DB lỗi. Red-team plan trước nhấn mạnh XSS. |
| D11 | Sensor cố định ở mọi viewport: `MouseSensor{distance:6}` + `TouchSensor{delay:250, tolerance:5}` (**không** `PointerSensor` — nó chiếm cử chỉ trước `TouchSensor`). Mobile (`< 768px`): DnD chỉ **sắp thứ tự trong cột**; đổi cột vẫn qua "Chuyển". Desktop: đổi cột + sắp thứ tự, có `DragOverlay`. | `BoardMobile` chỉ hiển thị một cột tại một thời điểm nên không có đích kéo sang cột khác; tablet cảm ứng ≥768px vẫn cần nhấn giữ; tránh xung đột với cuộn dọc. |
| D12 | Zod web: `description` giới hạn theo **văn bản thuần** `≤ 2000` ký tự (giữ như hiện tại) và HTML `≤ 20000`; server là thẩm quyền cuối. | Thông báo lỗi sớm cho người dùng; không nới lỏng so với v1. |
| D13 | **Không spread `attributes`** của dnd-kit lên card (dnd-kit luôn thêm `role="button"`, `aria-roledescription`, `aria-describedby`, `aria-pressed` dù truyền `undefined`); card chỉ nhận `setNodeRef` + `listeners` qua tham số `extra` của `getTaskProps` (lib merge ref). `KeyboardSensor` **không** đăng ký; live region của dnd-kit để **rỗng** (node vẫn tồn tại), thông báo đi qua live region sẵn có của trang. | Giữ nguyên hợp đồng listbox/option và test e2e hiện có (`getByRole("option")`); `ref` của lib (dời focus roving) không bị mất. |
| D14 | Test DnD: unit test hàm thuần (`resolveDrop`, `positionBetween`, map index → `after_task_id`) + Playwright kéo thật bằng `mouse.move` nhiều bước. Không giả lập kéo trong jsdom. | jsdom không có layout (`getBoundingClientRect` = 0) nên dnd-kit không kích hoạt được; e2e mới là bằng chứng thật. |

## Pattern map

| Việc cần làm | Mẫu có sẵn để theo |
|--------------|--------------------|
| Thêm port + fake trong core | `pkg/kanban/ports.go`, `service_test.go` (`fakeTaskRepo`, `fakeUoW`) |
| Transaction nhiều bước | `Service.ReorderColumns`/`DeleteColumn` dùng `s.uow.Within` |
| Lỗi 422 có `fields` | `translateError` trong `internal/features/tasks/errors.go` (`ErrAssigneeNotMember` → `fields.assignee_id`) |
| DTO tri-state | `Optional[T]` trong `dto.go` (dùng cho `after_task_id` nullable) |
| Query theo index cột | `taskRepository.MinPositionInColumn` dùng `idx_tasks_board (center_id, column_id, position)` |
| Migration + test parity | `migrations/000022_task_board.up.sql`, `migrations_test.go`, `backfill_parity_test.go` |
| Swagger | chú thích swag trên `handler.go` + `make api-docs` |
| Optimistic update web | `use-tasks-data-source.ts` (`onMutate` → `kanbanReducer`, `onError` → `invalidateQueries(tasksKeys.boards())`, scope `kanban-board`) |
| Presenter desktop/mobile | `board-desktop.tsx`, `board-mobile.tsx`, `task-column.tsx`, `task-card.tsx` |
| Form + lỗi server | `task-form-modal.tsx` (`applyKanbanFormError`, `Field`/`FieldLabel`/`FieldError`) |
| MSW + test | `src/test/msw/handlers.ts` (`POST /tasks/:id/move`), `features/tasks/__tests__/tasks-handlers.ts` |
| jsdom shim | `src/test/setup.ts` (ResizeObserver, pointer capture) |
| E2E | `apps/web/e2e/tasks-board.spec.ts` (login owner `0901000001`, `getByRole("listbox")`) |
| Hợp đồng API | `../260913-1102-task-center-kanban/reports/api-contract-v1.md` → viết `reports/api-contract-v2.md` |
| Nghiên cứu thư viện | `../reports/researcher-260917-1515-kanban-dnd-libraries.md`, `../reports/researcher-260917-1515-rich-text-editor-sanitizer.md` |

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Kéo thẻ việc sang cột khác và thả đúng vị trí; thứ tự bền vững sau reload | P1 |
| 2 | Sắp thứ tự trong cột bằng chuột (desktop) và cảm ứng (mobile) | P1 |
| 3 | Mô tả việc soạn bằng rich text; lưu HTML đã sanitize; render an toàn | P1 |
| 4 | Không phá hợp đồng a11y hiện có (`listbox`/`option`, `[`/`]`, "Chuyển") | P1 |
| 5 | Client cũ / API v1 body vẫn hoạt động (tương thích ngược) | P1 |
| 6 | Bundle bảng không phình: editor lazy, dnd-kit ~18 KB gzip | P2 |

## Phases

| # | Phase | Status | Effort | Depends |
|---|-------|--------|--------|---------|
| 1 | [API: move có vị trí (`after_task_id`) + renormalize](./phase-01-api-move-position.md) | Completed | 1d | — |
| 2 | [API: mô tả rich text (sanitize, giới hạn, migration 000023)](./phase-02-api-rich-text-description.md) | Completed | 1d | — |
| 3 | [Web lib `src/lib/kanban`: helper vị trí thuần, keyboard lên đầu cột](./phase-03-web-kanban-lib-position-helpers.md) | Completed | 0.5d | — |
| 4 | [Web: kéo-thả trong feature `tasks` (dnd-kit)](./phase-04-web-drag-and-drop.md) | Completed | 1.5d | 1, 3 |
| 5 | [Web: trình soạn thảo rich text (TipTap) + RichTextView](./phase-05-web-rich-text-editor.md) | Pending | 1.5d | 2 |
| 6 | [E2E, docs, hợp đồng v2, ship](./phase-06-e2e-docs-ship.md) | Pending | 1d | 4, 5 |

Phase 1, 2, 3 độc lập về file, có thể chạy song song. Phase 4 và 5 độc lập
nhau (file khác nhau trong `features/tasks`, trừ `task-form-modal.tsx` chỉ
phase 5 sửa và `task-card.tsx`/`task-column.tsx` chỉ phase 4 sửa).

## Acceptance Criteria

- [ ] **AC1** `POST /tasks/:id/move` với `{ column_id }` (không `after_task_id`) đặt việc lên đầu cột — y hệt v1; test HTTP cũ vẫn xanh không sửa.
- [ ] **AC2** `{ column_id, after_task_id }` đặt việc **ngay sau** `after_task_id` trong cột đích; `ListBoard` trả đúng thứ tự; hoạt động cả khi cùng cột (reorder) và khác cột.
- [ ] **AC3** `after_task_id` không tồn tại / khác tenant / không thuộc `column_id` / trùng chính việc đang chuyển → 422 `fields.after_task_id`.
- [ ] **AC4** Chèn liên tiếp vào cùng khe ≥ 60 lần không làm hỏng thứ tự (renormalize kích hoạt; integration test chứng minh thứ tự đúng và gap kề nhau ≥ 1e-6); 2 move đồng thời cùng `after_task_id` không tạo position trùng (advisory lock theo cột).
- [ ] **AC5** Mô tả gửi lên chứa `<script>`, `onclick`, `javascript:` href, `<img>` → lưu xuống chỉ còn allowlist; `<p></p>` → `""`; văn bản thuần `a\nb` → `<p>a<br>b</p>`; văn bản thuần > 4000 rune → 422 `fields.description`; body > 20000 rune → 422.
- [ ] **AC6** Migration 000023 up: mọi mô tả cũ không rỗng thành `<p>…</p>` đã escape (`<` → `&lt;`, `\n` → `<br>`); rỗng giữ rỗng; down chạy được không lỗi.
- [ ] **AC7** Desktop: kéo thẻ từ "Cần làm" thả giữa hai thẻ của "Đang làm" → thẻ nằm đúng vị trí ngay (optimistic) và sau reload (Playwright).
- [ ] **AC8** Mobile viewport: kéo giữ 250 ms sắp lại thứ tự trong cột; cuộn dọc bình thường không kích hoạt kéo; "Chuyển" menu vẫn là cách đổi cột.
- [ ] **AC9** Thẻ vẫn `role="option"` (không `role="button"`, `aria-roledescription`, `aria-describedby`, `aria-pressed`), cột `role="listbox"`, roving tabindex và `[`/`]` chạy như cũ (kể cả dời focus); test lib hiện có xanh (chỉ đổi kỳ vọng index 0); e2e cũ xanh.
- [ ] **AC10** Form tạo/sửa có toolbar (đậm, nghiêng, gạch chân, gạch ngang, ul, ol, link); lưu → mở lại giữ định dạng; người không phải creator thấy mô tả render read-only, không thấy toolbar.
- [ ] **AC11** Chunk editor không nằm trong chunk bảng (`npm run build` → chunk riêng chứa `@tiptap/*`, chunk trang bảng không import nó); test editor-trong-modal nằm ở `task-form-modal.test.tsx`, `task-board-page.test.tsx` không nạp `@tiptap/*`.
- [ ] **AC12** `make test-api`, `make lint-api`, `make api-docs` (diff sạch), `make test-web`, `make lint-web`, `make e2e-isolated` (4 spec `tasks-board*`, desktop + mobile project) đều xanh; `import_boundary_test` và ESLint override lib không đổi.

## Non-goals

- Kéo-thả **mô phỏng bằng bàn phím** (nhấc/thả bằng Space/Enter) — `[`/`]` vẫn là đường accessible.
- Kéo thẻ **sang cột khác trên mobile** (chỉ sắp thứ tự trong cột).
- Kéo-thả **cột** (sắp cột vẫn trong modal "Cấu hình cột").
- Ảnh, đính kèm, mention, bảng, tiêu đề, code block, markdown, chỉnh sửa cộng tác trong mô tả.
- Tìm kiếm/lọc theo nội dung mô tả.
- Khoá lạc quan / CAS khi hai người cùng sắp thứ tự (giữ D3 plan trước: last-write-wins + refetch).
- Đổi giới hạn 2000/4000 ký tự.

## Risks

| Rủi ro | Tín hiệu | Ứng phó đã chốt |
|--------|----------|-----------------|
| `@dnd-kit/core` 6.x không có release từ 12/2024; lỗi với React 19 StrictMode/Concurrent | `npm ls` cảnh báo peer, test smoke phase 4 fail, kéo không kích hoạt ở dev StrictMode | Toàn bộ dnd-kit nằm trong `use-board-dnd.ts` + 2 wrapper component; nếu vỡ, thay bằng `@dnd-kit/react` (cùng tác giả) mà không đụng lib/presenter khác. Replan phase 4 nếu cả hai vỡ. |
| Xung đột thuộc tính a11y dnd-kit với lib | Test e2e `getByRole("option")` fail hoặc axe báo `role` trùng | D13: không spread `attributes` của dnd-kit; `setNodeRef` + `listeners` đi qua `extra` của `getTaskProps`; test unit assert `role="option"`, không `aria-*` lạ, `]` vẫn dời focus. |
| Hai move đồng thời vào cùng cột trùng position / renormalize đan xen | Integration test 2 goroutine thấy position trùng | Phase 1: `pg_advisory_xact_lock(center_id, column_id)` ngay đầu `ListColumnPositions` trong tx; các move cùng cột tuần tự hoá. |
| Nhấn giữ trên mobile không kích hoạt kéo | E2E mobile (CDP touch) không đổi thứ tự; kéo nhanh lại kích hoạt | D11: `MouseSensor` + `TouchSensor`, không `PointerSensor`; e2e mobile là gate. |
| Lệch 1 vị trí khi kéo xuống cùng cột | Optimistic nhảy ngược so với hình ảnh khi kéo; e2e AC7 thứ tự sai | Phase 3: `resolveDrop` theo `arrayMove` (cùng cột → `index = overIndexGốc`); test kỳ vọng mảng kết quả cụ thể. |
| Sanitize không đủ (bypass bluemonday) | Test AC5 fail với payload mới; advisory CVE | Allowlist tối thiểu + `AllowStandardURLs` + `RequireNoFollowOnLinks`; DOMPurify phía client (D10) là lớp hai. Theo dõi `govulncheck`. |
| Migration 000023 sai trên dữ liệu thật (ký tự lạ, mô tả có `<` hợp lệ) | Bảng hiện `&lt;` thô hoặc thẻ trần sau deploy | Backup trước; `backfill_parity_test`-style test với fixture chứa `<`, `&`, `\r\n`; rollback bằng restore backup (down lossy chỉ dùng dev). |
| Float precision: hai client cùng chèn vào một khe | Thứ tự hiển thị khác nhau giữa hai client | Server luôn quyết định; client refetch sau lỗi; renormalize khi gap < 1e-6 (AC4). |
| TipTap trong jsdom thiếu `getClientRects`/`Range` | Test phase 5 ném `TypeError` | Thêm shim vào `src/test/setup.ts` (mẫu remirror jsdom-polyfills); test dùng `editor.commands` thay vì gõ phím. |
| Bundle phình | `npm run build` báo chunk bảng tăng > 30 KB gzip | Editor `React.lazy`; kiểm `build:analyze` ở phase 6. |
| Cuộn dọc mobile bị chặn bởi TouchSensor | Người dùng không cuộn được cột | `delay: 250, tolerance: 5` + `touch-action: manipulation` trên thẻ (dnd-kit tự `preventDefault` touchmove sau khi activate); e2e mobile viewport kiểm cuộn. |
| Toolbar TipTap không cập nhật trạng thái | Nút Đậm không sáng sau khi bấm | D9: `useEditorState` selector; test assert `aria-pressed`. |

## Open questions

Không có câu hỏi chặn. Hai điểm đã tự chốt và ghi ở D2 (đường không kéo-thả
vẫn lên đầu cột) và D11 (mobile chỉ sắp trong cột); nếu người dùng muốn khác,
sửa một hằng số trong phase 3/4, không đổi kiến trúc.

## Rollback

- **API**: `after_task_id` là optional nên revert code không ảnh hưởng client.
  Migration 000023 **không** có down an toàn cho dữ liệu đã soạn rich text →
  rollback bằng restore backup DB đã tạo trước khi deploy (phase 6).
- **Web**: revert commit phase 4/5; API vẫn nhận body v1. Mô tả đã lưu HTML
  sẽ hiện thẻ thô trên web cũ → chỉ rollback web cùng lúc với API + restore DB,
  hoặc giữ API v2 và chỉ tắt DnD (feature flag không có; revert presenter).
- Mỗi phase là commit riêng, conventional commit, không tham chiếu AI.

## Red Team Review

Hai reviewer đối kháng chạy song song trên plan + code thật + tarball thư viện,
báo cáo tại
[redteam-260917-1515-api-move-rich-text.md](../reports/redteam-260917-1515-api-move-rich-text.md)
(API: 1 High, 5 Medium, 8 Low) và
[redteam-260917-1515-web-dnd-rich-text.md](../reports/redteam-260917-1515-web-dnd-rich-text.md)
(web: 4 High, 8 Medium, 6 Low). Không có Critical. Mọi phát hiện đã được kiểm
lại trên mã nguồn (`use-kanban-keyboard.ts:275-293`, `task-column.tsx:22,88`,
`task-board-page.tsx:45`, `Makefile:88`, `playwright.config.ts`,
`src/styles/globals.css`) trước khi nhận.

| Mã | Phát hiện | Xử lý |
|----|-----------|-------|
| API H1 | `WithinTx` READ COMMITTED, không khoá → 2 move đồng thời trùng position | **Nhận.** Phase 1: `pg_advisory_xact_lock(center_id, column_id)` đầu `ListColumnPositions`; test 2 goroutine; AC4 + Risks cập nhật. |
| API M2 | `tasks.Service.Board` `sort.Slice` không ổn định | **Nhận.** Phase 1: `sort.SliceStable`. |
| API M3 | `after_task_id` không phải UUID → 400 BindError, không 422 | **Nhận.** Phase 1 ghi rõ 400 vs 422, test riêng. |
| API M4 | Bộ fake thứ hai ở feature test + 5 test core gọi `MoveTask` phải sửa chữ ký | **Nhận.** Phase 1 liệt kê cả hai file; bỏ câu "test hiện có không sửa". |
| API M5 | Guard `NOT LIKE '<p>%'` bỏ sót mô tả thuần bắt đầu bằng `<p>` | **Nhận.** Phase 2 + D8: bỏ guard, thêm post-check `count(*) = 0`. |
| API M6 | `normalizeDescription` không bọc plain text → mất xuống dòng | **Nhận.** Phase 2 + D7: input không bắt đầu bằng `<` → escape + `<br>` + `<p>` trước sanitize; AC5 thêm case. |
| API L7–L14 | bluemonday bỏ hẳn `<a>`; `max=` đếm rune; AC4 "số nguyên" sai; README use-case cũ; backup trước `compose up`; e2e không cần backup; thứ tự unescape down; scopelint không soi port mới | **Nhận cả 8.** Sửa chữ trong phase 1/2/6, D7/D8, AC4/AC5. |
| Web H1 | `ref={mergeRefs(setNodeRef)}` bị `taskProps.ref` ghi đè; `mergeRefs` private | **Nhận.** D13 + phase 4: `getTaskProps(id, { ref: setNodeRef, ...listeners })`; nới kiểu ở 3 component; test `]` dời focus. |
| Web H2 | `attributes: { role: undefined }` không tắt được `role="button"`/`aria-*` | **Nhận.** D13 + phase 4: không spread `attributes`; AC9 liệt kê 4 thuộc tính cấm. |
| Web H3 | `resolveDrop` "chèn trước over" lệch 1 khi kéo xuống cùng cột | **Nhận.** Phase 3: quy tắc `arrayMove`, test kỳ vọng mảng cụ thể; Risks mới. |
| Web H4 | `PointerSensor` chiếm cử chỉ trước `TouchSensor` → nhấn giữ chết; tablet cảm ứng nhận sensor desktop | **Nhận.** D11 + phase 4: `MouseSensor` + `TouchSensor` mọi viewport; `touch-action: manipulation`; e2e mobile là gate. |
| Web M1 | dnd-kit luôn render live region; "chỉ 1 aria-live" không thể đạt | **Nhận.** AC9 + phase 4: live region dnd-kit rỗng, test chọn theo text. |
| Web M2 | `click` compat trên cảm ứng đến sau 50 ms → mở modal sau khi thả; nút menu chỉ chặn pointerdown | **Nhận.** Phase 4: `justDroppedRef`; chặn `mousedown`+`touchstart`; e2e mobile assert không dialog. |
| Web M3 | TipTap v3 không re-render theo transaction → `aria-pressed` cũ | **Nhận.** D9 + phase 5: `useEditorState`; test assert `aria-pressed`. |
| Web M4 | `@tiptap/extension-character-count` là shim deprecated; researcher nói thiếu `extension-link` là lỗi thời | **Nhận.** Phase 5: `@tiptap/extensions`; ghi chú researcher lỗi thời. |
| Web M5 | `src/index.css` và Popover wrapper không tồn tại | **Nhận.** Phase 5: `src/styles/globals.css`; ô URL inline thay popover (không thêm primitive cho một chỗ dùng). |
| Web M6 | AC11 mâu thuẫn với test trong `task-board-page.test.tsx` | **Nhận.** AC11 + phase 5: tách `task-form-modal.test.tsx`. |
| Web M7 | `make e2e` không dựng stack cô lập; Playwright chưa có `projects` | **Nhận.** Phase 6: lệnh thật + `make e2e-isolated`; `testIgnore` cho desktop; gate = 4 spec `tasks-board*` (3 suite cũ đỏ sẵn từ 01/09). AC12 + Environment cập nhật. |
| Web M8 | Thiếu file test bị ảnh hưởng; optimistic của `deleteColumn` lệch tạm | **Nhận.** Phase 4 liệt kê `use-kanban-keyboard.test.tsx`, `use-tasks-data-source.test.tsx`; `deleteColumn` để nguyên (refetch sửa). |
| Web L1–L6 | position âm ok; DOMPurify hook đăng ký module scope; `defaultValues` qua `normalizeIncoming`; polyfill đủ; drag helper ok nếu H4 sửa; số dòng | **Nhận L1, L2, L3, L5** (sửa chữ phase 3/5/6); L4, L6 không cần sửa. |

Reviewer xác nhận không có vấn đề: thuật toán midpoint/1e-6, lọc `after_task_id`
theo tenant/cột, thứ tự `translateError`, tương thích body v1 qua `Optional[T]`,
allowlist bluemonday khớp TipTap, thứ tự escape trong migration, harness test
migration, một caller `MoveTask`, import boundary; zod refine theo plain text,
`KeyboardSensor` không cần, `BoardMobile` một cột, `over === active` → null,
Playwright `workers: 1` tương thích `projects`.

## Whole-Plan Consistency Sweep

Rà sau khi gộp red-team (17/09):

- **Effort**: 1 + 1 + 0.5 + 1.5 + 1.5 + 1 = 6.5d, khớp frontmatter và bảng
  Phases. Không tăng dù thêm advisory lock/`useEditorState` (đều nhỏ).
- **Dependencies**: phase 4 ← 1, 3; phase 5 ← 2; phase 6 ← 4, 5; khớp
  frontmatter từng phase. Phase 1/2/3 độc lập về file.
- **AC ↔ phase**: AC1–AC4 → phase 1; AC5–AC6 → phase 2; AC7–AC9 → phase 3/4;
  AC10–AC11 → phase 5; AC12 → phase 6. Mỗi AC xuất hiện trong Success Criteria
  của đúng phase.
- **Thuật ngữ thống nhất**: `MoveTask(ctx, tenant, actor, id, target, after)`;
  `after_task_id` (API) ↔ `afterTaskId` (web); `index` = chỉ số đích sau khi
  loại task đang kéo, quy tắc `arrayMove` cho cùng cột; sensor
  `MouseSensor`+`TouchSensor`; `getTaskProps(id, extra)`; `make e2e-isolated`;
  `src/styles/globals.css`; `@tiptap/extensions`; giới hạn "20000 rune" HTML,
  "4000 rune" / "2000 ký tự" văn bản thuần.
- **Tên file**: 6 phase file khớp bảng Phases; report red-team/researcher link
  tương đối từ `plan.md` kiểm được; `reports/api-contract-v2.md` tạo ở phase 6.
- **Backup DB**: chỉ prod (D8, phase 2, phase 6), trước `compose up`; dev/e2e
  seed lại nên không cần.
- **Không còn** tham chiếu `PointerSensor` (ngoài D1 mô tả năng lực thư viện
  và câu cấm), `mergeRefs`, `pan-y`, `index.css`, `extension-character-count`,
  guard `NOT LIKE` trong lệnh migration, "chỉ 1 vùng aria-live".

<!-- slug: task-dnd-rich-text -->
