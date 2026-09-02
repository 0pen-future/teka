---
title: "Chấm điểm thành phần và bộ điểm — redesign web-only"
description: "Dựng lại màn chấm điểm thành phần (tab Điểm buổi) và màn định nghĩa/gán bộ điểm theo report ui-redesign-260902-1029, làm nhất quán với hv kit trước rồi mới mở rộng UI; không đổi API/DB."
status: completed
priority: P1
effort: "6-7d"
tags: [web, ui, hv-kit, grading, teaching, center]
created: 2026-09-02
blockedBy: []
blocks: []
---

# Chấm điểm thành phần và bộ điểm — redesign web-only

## Overview

Nguồn: `plans/reports/ui-redesign-260902-1029-session-scoring-and-score-sets.md`
(chẩn đoán S1–S9, E1–E9, thiết kế mục 4–5) và các phát hiện nhất quán liên
quan trong `plans/reports/ui-review-260902-1001-web-ui-consistency.html`
(C2, C3, C4, C6, C8, C9, C10). Mockup: `plans/reports/ui-redesign-260902-1029-session-scoring-and-score-sets.html`.

Thứ tự theo yêu cầu: **nhất quán UI/UX trước** (bổ sung primitive vào hv kit
và áp cho các màn đang có), **rồi mới mở rộng UI cho bộ điểm và chấm điểm**.

**Phạm vi web-only, đã xác minh với API/DB** (`apps/api/internal/features/grading/dto.go`,
`apps/api/migrations/000014_grading.up.sql`):

- `GET /score-sets` trả `{id, name, components: string[]}`; `GET /classes/:id/score-components`
  trả `{class_id, components: [{id, name, position}]}`; `GET /sessions/:id/scores` trả
  `{components, scores: [{student_id, component_id, score}]}`; `PUT /sessions/:id/scores`
  nhận mảng `{student_id, component_id, score | null}` (null = xoá ô). Tất cả đủ cho mục 4 và 5.
- API **không** trả `has_scores` hay `class_count`, và không expose `source_set_id`
  (`class_score_components.source_set_id` chỉ có ở DB). Hai mục ở "API tuỳ chọn" (report §7)
  vì vậy là **non-goal**; web dùng phương án thay thế (xem Quyết định D4, D5).
- Service chấm điểm chỉ kiểm tra học sinh có trong roster ngày đó (`RosterSource.ActiveOn`),
  không kiểm tra trạng thái điểm danh; quy tắc "chỉ chấm học sinh có mặt" là quy tắc web.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | hv kit có đủ primitive cho hai màn (HvModal size, HvScoreInput, HvSegmented, HvNotice, HvConfirmDialog, HvStateBlock), có test | P1 |
| 2 | Các file chấm điểm/bộ điểm hiện có dùng đúng kit: không còn nút/tab thô, `type=number`, `save-button-styles`, chuỗi "Đang tải…" tự viết, `th` thiếu scope | P1 |
| 3 | Tab Điểm buổi chấm theo học sinh trong panel 400px không cuộn ngang; dưới `sm` là bottom sheet | P1 |
| 4 | Bảng đầy đủ trong modal xl: tiêu đề dính, cột tên dính, Enter/Tab điều hướng, cột TB, hàng vắng gộp cuối | P1 |
| 5 | Trình soạn bộ điểm: modal lg, hàng 48px, lỗi theo hàng, dán danh sách, đếm n/10, xem trước tiêu đề | P1 |
| 6 | Danh sách bộ điểm dạng thẻ + chip cột; gán bộ điểm bằng radio card thấy cột trước khi gán; bảng gán có bản mobile | P2 |

## Constraints

- Không đổi API, DB. Không đổi **shape wire** của schema zod (`features/center/schemas/grading.ts`,
  `features/teaching/schemas/teaching-schemas.ts`); refactor nội bộ `superRefine` (tách helper, giữ message/path) được phép.
- Giữ nguyên design system: chỉ dùng token trong `src/styles/tokens`, không thêm màu/bán kính/font.
- Cross-feature import chỉ qua `index.ts` (frontend-guidelines); primitive dùng chung đặt ở `components/hv`.
- Mọi ô nhập và nút ≥ 44px; không còn `type="number"` trong hai màn này.
- Test: Vitest + MSW offline (`npm run test` trong `apps/web`), `npm run lint`, `npm run typecheck`.
  Mở rộng test hiện có (`component-score-grid.test.tsx`, `class-config-page.test.tsx`, `hv-modal.test.tsx`).

## Non-goals

- Thêm `class_count` vào `GET /score-sets` và `has_scores` vào `GET /classes/:id/score-components` (report §7). Ghi lại làm follow-up API riêng.
- Hiển thị bộ điểm đang gán / số cột của **từng lớp** trong bảng lớp ở trang cấu hình: cần endpoint batch (hiện chỉ có `GET /classes/:id/score-components` từng lớp → N+1). Follow-up API.
- Trong trình soạn bộ điểm: bấm tiêu đề để focus ô, Enter ở hàng cuối tự thêm hàng, ẩn nút Thêm khi đủ 10 cột (giữ nút disabled + helper).
- Quét nhất quán 27 trang của review tổng (Đợt 1–3). Primitive tạo ở Phase 1 được thiết kế để đợt đó tái dùng, nhưng việc áp vào trang khác là plan khác.
- Mẫu bộ điểm gợi ý (chip "4 kỹ năng IELTS"…): câu hỏi mở của report, chưa chốt → không làm.
- Trọng số / điểm tối đa theo cột (đổi schema).
- Thư viện kéo thả (`@dnd-kit`): không thêm dependency cho danh sách tối đa 10 hàng.
- Đưa điểm thành phần vào báo cáo phụ huynh.

## Decisions

| # | Quyết định | Căn cứ |
|---|-----------|--------|
| D1 | Giữ nút "Lưu điểm" **và** tự lưu khi rời ô (blur) sau 800ms qua `useDebouncedSave`; nút chỉ để flush ngay; một toast mỗi đợt flush. | Report §4.3; API ghi đè từng ô và nhận `null` nên không mất dữ liệu. Câu hỏi mở của report được chốt theo đề xuất của report. |
| D2 | Học sinh `late` được chấm điểm như `present` (điều kiện `held && (present \|\| late)`). Học sinh `absent`/`excused` đã có điểm lưu từ trước vẫn hiển thị điểm read-only trong nhóm Vắng, không xoá. | Grid hiện tại viết trước khi có 4 trạng thái; API không chặn theo điểm danh. Học sinh đi muộn vẫn làm bài. Thay đổi hành vi nhỏ, có test. **Giả định cần chủ sản phẩm xác nhận trước khi merge Phase 3**; nếu bác, đổi một dòng điều kiện trong `useScoreDraft`. |
| D3 | Sắp xếp cột bằng nút ↑ ↓ 44px (HvButton ghost, icon) + chế độ "Dán danh sách" thay cho kéo thả. Giữ `watch`/`setValue` trên `string[]`, không chuyển `useFieldArray`. | Non-goal thư viện DnD; danh sách ≤10; dán danh sách giải quyết E4 và thứ tự cùng lúc. `useFieldArray` đòi đổi shape schema sang object (research set-editor §1). |
| D4 | Khoá sớm khi lớp đã có điểm: web **không** biết trước 409. Dialog gán hiện HvNotice info khi mở ("Lớp đã có điểm sẽ không đổi được bộ điểm"); sau 409 thì khoá dialog và nhớ `lockedClassIds` trong state trang cho tới khi rời trang. | Không đổi API. Hành vi hiện tại giữ nguyên, chỉ báo sớm hơn bằng lời. |
| D5 | Bỏ dòng "Đang dùng ở N lớp" và việc chặn xoá bộ đang dùng. | Cần `class_count`/`source_set_id` từ API. Snapshot nghĩa là xoá bộ gốc không ảnh hưởng lớp đã gán, nên không có rủi ro dữ liệu. |
| D6 | `HvModal` thêm prop `size: "md" \| "lg" \| "xl"`; md = 448px hiện tại, lg = 720px, xl = `--w-page` 1080px cao 90dvh. Dưới `sm` cả ba vẫn là bottom sheet, xl dùng `max-h-[95dvh]`. | Report §4.4, §5.1. |
| D7 | Dưới `sm`, panel chi tiết buổi mở trong HvModal (bottom sheet) thay vì xếp dưới bảng buổi; thêm `useMediaQuery` (useSyncExternalStore) vào `lib/hooks`, query mobile là `(max-width: 639px)`. | Report §4.1 (S8). Chưa có hook media query trong repo; test setup stub `matchMedia` trả `matches:false` cho mọi query, nên query dạng `max-width` giữ jsdom ở chế độ desktop mặc định. |
| D8 | `ComponentScoreGrid` được thay bằng `ScoreEntryByStudent` (panel) + `ScoreTableModal` (xl), dùng chung `useScoreDraft`. Ô điểm chung (general score) cũng dùng `HvScoreInput` nhưng giữ luồng lưu tay hiện có; ô rỗng vẫn bị bỏ qua (không gửi `null`, không xoá điểm chung). | Report §4.4. Không đụng endpoint marks; `MarkEntryInput.score` tri-state, xoá điểm chung ngoài scope. |
| D9 | Bảng điểm dùng `<table>` thường với input có label, không `role="grid"`; điều hướng bằng refs matrix theo `cellKey`. Ô nhập `type="text" inputmode="decimal"`. | Research grid-ux §1–2: `role=grid` phải tự dựng roving tabindex; `type=number` bị scroll-wheel đổi giá trị và từ chối dấu phẩy. |

## Phases

| # | Phase | Status | Effort | Depends |
|---|-------|--------|--------|---------|
| 1 | [Kit foundations](./phase-01-kit-foundations.md) | Completed | 1d | — |
| 2 | [Consistency pass on existing scoring screens](./phase-02-consistency-pass.md) | Completed | 0.5d | 1 |
| 3 | [Score entry by student (panel + mobile sheet)](./phase-03-score-entry-by-student.md) | Completed | 1.5d | 1, 2 |
| 4 | [Full score table modal (xl)](./phase-04-score-table-modal.md) | Completed | 1d | 3 |
| 5 | [Score set editor](./phase-05-score-set-editor.md) | Completed | 1d | 1, 2 |
| 6 | [Score set list and assign dialog](./phase-06-score-set-list-and-assign.md) | Completed | 1d | 1, 2, 5 |

Phase 3–4 (teaching) và 5–6 (center) độc lập về file sau Phase 2; có thể chạy song song trên hai nhánh.

## Dependency map

```
P1 kit ──► P2 consistency ──┬──► P3 by-student ──► P4 table modal
                            └──► P5 editor ──► P6 list + assign
```

Ranh giới file: P3–P4 chỉ sửa `features/teaching/**` + `lib/hooks` (+ fixture test `features/roster/__tests__/roster-handlers.ts` để seed `late`/`excused`); P5–P6 chỉ sửa `features/center/**`.
`components/hv/**` chỉ sửa ở P1 (và sửa nhỏ nếu P3–P6 phát hiện thiếu prop, ghi rõ trong PR).

## Success Criteria

- [x] Bộ điểm 8 cột, lớp 20 học sinh: chấm đủ trong panel ~400px không cuộn ngang (test jsdom: không có `overflow-x-auto` bao bảng trong panel; kiểm tra tay ở 1080px). — *jsdom đã kiểm; ảnh 1080px với 10 cột: `scores-panel-1080.png` (stack e2e cô lập chỉ có 2 học sinh seed).*
- [x] Bảng đầy đủ ở 1280px không cuộn ngang với 10 cột (180 + 10×72 + 72 = 972px < ~1032px nội dung modal 1080px). — *đã kiểm tay: `score-table-1280.png`, `score-table-1080.png` trong `plans/reports/screenshots-260902-scoring-ui/`; bảng đo đúng 972px, sticky header/cột tên và line-clamp xác nhận bằng computed style.*
- [x] Mọi input điểm và nút trong hai màn ≥ 44px; `grep -rn 'type="number"'` không còn trong `features/teaching` và `features/center`.
- [x] Enter đi xuống cùng cột, Tab sang phải, Shift+Enter đi lên trong bảng đầy đủ; tiêu đề cột luôn hiện khi cuộn dọc.
- [x] Ô chưa lưu có nền sun-100 + `data-state="dirty"`; thanh trạng thái đếm "n ô chưa lưu"; đóng panel, đổi buổi hoặc đổi lớp khi còn ô chưa lưu thì hộp thoại xác nhận hỏi (dùng `HvModal` ba nút thay `HvConfirmDialog`; modal bảng đầy đủ không guard — xem Deviations).
- [x] Tạo bộ 8 cột bằng dán 8 dòng trong một thao tác; lỗi trùng hiện đúng dưới hàng trùng.
- [x] Dialog gán hiện chip cột của bộ được chọn trước khi bấm Gán; 409 vẫn khoá với thông điệp hiện có.
- [x] `save-button-styles.ts`, `component-score-grid.tsx` bị xoá; không file nào trong hai màn tự viết "Đang tải…".
- [x] `npm run lint`, `npm run typecheck`, `npm run test` xanh; test hiện có được cập nhật, không bị xoá để lách.

## Deviations khi thực thi (2026-09-02)

- Guard "còn ô chưa lưu" là `HvModal` với ba nút "Ở lại" / "Bỏ thay đổi" / "Lưu và đóng" (focus mặc định vào "Lưu và đóng", hoặc "Ở lại" khi còn ô không hợp lệ để Enter không bao giờ xóa nháp), không phải `HvConfirmDialog`, vì cần nhiều hơn một hành động khẳng định.
- `onDirtyChange` của `ScoreEntryByStudent` truyền **số ô** dirty và số ô không hợp lệ (không phải boolean) để guard hiện "Còn n ô chưa lưu" và, khi còn ô không hợp lệ, vô hiệu "Lưu và đóng" kèm thông báo — `flush()` trả `false` khi còn ô invalid vì ô đó không được gửi (phát hiện ở code review).
- `flush()` commit trước mọi ô còn ở trạng thái `editing` (input bị unmount trước blur, ví dụ đóng bảng đầy đủ bằng Esc) để chữ đã gõ được gửi hoặc báo không hợp lệ, không bị bỏ lặng (phát hiện ở re-review).
- Modal "bảng đầy đủ" **không** có guard khi đóng: nháp dùng chung với panel nên đóng modal không mất gì; guard chỉ ở panel (đóng panel, đổi buổi, đổi lớp — đổi lớp cũng đi qua guard, spec ban đầu chỉ ghi đổi buổi). Trang reset đếm ô dirty khi tự đóng panel vì panel unmount trước khi effect báo về.
- Điểm trung bình hiển thị `toFixed(1)` ("7.5") thống nhất với phần còn lại của app, không dùng "7,5" như mockup.
- Memo hàng bảng bằng comparator `areRowPropsEqual` (`hooks/use-row-cells.ts`) thay vì mảng ổn định qua ref: React Compiler cấm ghi ref trong render.
- Payload autosave được dựng **tại thời điểm debounce bắn** (lazy) để ô đã revert trong 800ms không bị gửi lại — phát hiện qua test, ngoài spec.
- Test trình soạn bộ điểm đặt ở `score-set-editor-modal.test.tsx` riêng (spec ghi `class-config-page.test.tsx`) để file page test không phình.
- `components/ui` không có `Textarea` nên chế độ dán dùng `<textarea>` thường với class lặp của `Input`.
- Kiểm tra tay 1080/1280/390px làm bằng Playwright trên stack e2e cô lập (`docker compose -p teka-e2e`, seed dev), ảnh và `findings*.txt` ở `plans/reports/screenshots-260902-scoring-ui/`. Lưu ý khi tự chụp: đổi viewport qua mốc 639px sẽ unmount panel (mất nháp) — vấn đề đã ghi ở Open questions; ở 390px modal bảng đầy đủ mở chồng lên sheet, bảng cuộn ngang trong thân modal (chấp nhận, không cuộn ngang trang).

## Validation (câu hỏi kiểm định)

| Câu hỏi | Trả lời | Bằng chứng |
|---------|---------|------------|
| Có cần đổi API/DB không? | Không. Mọi màn dùng 6 endpoint hiện có; hai mục cần API bị loại thành non-goal. | `apps/api/internal/features/grading/dto.go`; `apps/web/src/features/center/api/grading.ts` |
| Thay đổi hành vi nào người dùng sẽ thấy? | (1) học sinh `late` chấm được; (2) chữ vô nghĩa trong ô điểm báo lỗi thay vì bị bỏ; (3) tự lưu khi rời ô; (4) xoá bộ điểm qua dialog; (5) mobile: panel thành bottom sheet. | D1, D2, Phase 2–3 |
| Có thể rollback từng phase không? | Có: mỗi phase là một PR, kit primitive (P1) không phá màn cũ; P3 xoá `component-score-grid.tsx` là điểm không quay lại bằng revert PR đơn lẻ. | Dependency map |
| Test hiện có bị xoá không? | Không xoá; `component-score-grid.test.tsx` được đổi tên và viết lại theo UI mới, giữ fixture. | Phase 3 |
| Điều gì chưa kiểm được bằng jsdom? | Sticky/line-clamp/kích thước pixel; bù bằng kiểm tra tay có ảnh trong PR ở 1280/1080/390px. | Phase 3, 4, 6 Success Criteria |
| Điểm rủi ro cao nhất? | Race giữa debounce blur và mutation đang bay trong `useScoreDraft`; `schedule` thay payload nên phải snapshot toàn bộ dirty; `discard` phải `cancel` trước vì hook flush khi unmount. Ba test fake-timers bắt buộc. | Phase 3 Risk |
| Đã red-team chưa? | Có: `plans/reports/red-team-260902-1209-session-scoring-ui-redesign.md`, GO WITH FIXES; C1–C5, M1–M11 và các minor đã vá vào phase files. | Report red-team |

## Open questions

- Re-review còn để lại mức Medium, chưa xử lý trong đợt này: lật breakpoint 639px khi panel đang mở làm unmount panel (mất ô chưa commit); điều hướng router không đi qua guard (chỉ `beforeunload`); đếm ô dirty ở trang không reset nếu buổi biến mất sau refetch; khi có ô không hợp lệ không có lối lưu riêng các ô hợp lệ.

- Mẫu bộ điểm gợi ý (non-goal hiện tại) có muốn làm ở đợt sau không?
- Follow-up API (`class_count`, `has_scores`, batch `score-components` cho bảng lớp): có mở plan riêng không?
- (D2 và D8 là giả định đã chốt trong bảng Quyết định; chủ sản phẩm có thể bác trước khi merge Phase 3/2.)

## Research

- `plans/reports/research-grid-ux-260902-1209-score-grid-ux.md` — bảng thường + refs matrix (không `role=grid`); `type=text inputmode=decimal`; sticky corner z-index; autosave blur + flush, guard `isPending`; `aria-invalid` + text. **Hai điểm sai đã được red-team sửa**: `useDebouncedSave` hiện không có caller production (chỉ test của chính nó), và TanStack `mutate` không xếp hàng mutation.
- `plans/reports/research-set-editor-260902-1209-score-set-editor-patterns.md` — giữ `watch/setValue` (không `useFieldArray`); nút ↑↓ thay dnd-kit; dán danh sách là hai view trên cùng state; Radix RadioGroup card; bảng responsive dùng hai markup, không CSS reflow.
- `plans/reports/red-team-260902-1209-session-scoring-ui-redesign.md` — red-team, GO WITH FIXES; mọi phát hiện C/M/m đã vá.

<!-- slug: session-scoring-ui-redesign -->
