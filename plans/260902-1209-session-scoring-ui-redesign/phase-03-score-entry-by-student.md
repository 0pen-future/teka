---
phase: 3
title: "Score entry by student (panel + mobile sheet)"
status: completed
priority: P1
effort: "1.5d"
dependencies: [1, 2]
---

# Phase 3: Score entry by student (panel + mobile sheet)

## Overview

Thay `ComponentScoreGrid` trong tab "Điểm buổi" bằng chế độ **theo học sinh**
(report §4.1): danh sách hàng 56px mở rộng thành các ô điểm 44px, tiến độ n/N,
ô chưa lưu tô sun-100, thanh dưới dính với "Lưu điểm", tự lưu khi rời ô, hỏi
xác nhận khi đóng còn ô chưa lưu. Dưới `sm` panel mở dạng bottom sheet. Logic
nháp/lưu tách ra `useScoreDraft` để Phase 4 dùng lại. Phase này cũng là nơi
**xoá** `component-score-grid.tsx`, `save-button-styles.ts` và `parseScoreInput`
cũ trong `classbook-stats.ts` (caller cuối cùng biến mất ở đây, không phải Phase 2).

## Requirements

- Functional:
  - Hàng học sinh: tên, `display_note`, StatPill trung bình (nếu có ít nhất một ô), số ô đã chấm `k/M`; bấm để mở rộng; chỉ **một hàng mở** (accordion), mở hàng tiếp theo bằng Enter ở ô cuối.
  - `present` và `late` chấm được (D2). `absent`/`excused` gộp xuống cuối trong nhóm "Vắng (n)" thu gọn (`<details>`), **không có ô nhập**; học sinh vắng nhưng đã có điểm lưu từ trước (chấm trước khi đổi điểm danh) vẫn hiển thị điểm dạng text read-only trong nhóm này, không xoá, không cho sửa.
  - Hàng không chấm được (`canWrite=false`, hoặc buổi chưa `held`) hiển thị điểm đã lưu dạng text read-only cùng bố cục hàng; buổi chưa `held` giữ copy "Chấm điểm sau khi buổi diễn ra".
  - Ô nhập: `HvScoreInput` 44px; Enter → ô kế trong hàng, ô cuối → mở hàng tiếp; Shift+Enter ngược lại; Tab theo DOM.
  - Ô sửa mà chưa lưu: `state="dirty"`; sau lưu thành công `state="saved"` 1.5s rồi về idle.
  - Thanh dính đáy: "12/18 học sinh đã chấm · 3 ô chưa lưu" + `HvButton` "Lưu điểm" (disabled khi không dirty, có ô invalid, hoặc đang lưu).
  - **Tự lưu**: sau khi commit một ô dirty, `schedule()` với **snapshot toàn bộ tập dirty** (`buildPayload()` tại thời điểm gọi), không phải chỉ ô vừa blur — vì `useDebouncedSave.schedule` thay thế payload đang chờ (`use-debounced-save.ts`), gọi với một ô sẽ làm rơi ô blur trước đó. Nút Lưu = `flush()`. Không schedule khi mutation đang bay (`isPending`); `onSettled` còn dirty thì schedule lại.
  - **Bỏ nháp**: `discard()` phải gọi `cancel()` của `useDebouncedSave` **trước** khi xoá dirty, vì hook flush khi unmount (`use-debounced-save.ts:60`); nếu không, "Bỏ thay đổi" rồi đóng panel vẫn PUT.
  - Đóng panel / đổi tab khi còn dirty → `UnsavedScoresGuard` ("Còn n ô chưa lưu": "Lưu và đóng" / "Bỏ thay đổi"). `beforeunload` khi dirty.
  - **Đổi buổi khi dirty**: `key={selectedSessionId}` unmount panel, nên panel không thể tự chặn. Luồng: panel báo `onDirtyChange(isDirty)` lên `ClassbookPage`; page giữ `pendingSessionId`; khi chọn buổi khác mà panel dirty, page mở guard trước, chỉ `setSelectedSessionId` sau "Lưu và đóng" (page gọi `panelRef.current.flush()` qua `useImperativeHandle`) hoặc "Bỏ thay đổi" (`discard()`).
  - Nút "Mở bảng đầy đủ" chỉ xuất hiện ở Phase 4; phase này không render nút chết.
  - Dưới `sm`: `ClassbookPage` render `SessionDetailPanel` trong `HvModal size="md"` (bottom sheet) thay vì cạnh bảng; tiêu đề modal = nhãn buổi; `onOpenChange(false)` cũng đi qua guard.
- Non-functional: không cuộn ngang trong panel; jsdom không đo pixel; hàng học sinh bọc `React.memo` với props nguyên thuỷ (raw/state của các ô trong hàng) để gõ một ô không re-render 20 hàng.

## Architecture

```
src/lib/hooks/use-media-query.ts      dùng chung app
features/teaching/
  hooks/use-score-draft.ts        state + save orchestration (dùng chung P3/P4)
  components/score-entry-by-student.tsx   UI accordion; export type ScoreEntryHandle {flush, discard}
  lib/score-entry-summary.ts              tính k/M, avg, dirtyCount (hàm thuần)
  components/score-entry-footer.tsx       thanh trạng thái + nút Lưu (dùng chung P3/P4)
  components/unsaved-scores-guard.tsx     HvConfirmDialog với copy chuẩn (dùng chung P3/P4)
  components/session-detail-panel.tsx     dùng ScoreEntryByStudent; guard cho close/tab; forwardRef {flush, discard, isDirty}
  pages/classbook-page.tsx                mobile: HvModal bọc panel; pendingSessionId + guard khi đổi buổi
```

`useScoreDraft(sessionId, {components, rosterRows, canWrite, held})`:
- `cells: Map<cellKey, {raw, server: number|null, state}>` dựng từ `useSessionScores` + draft.
- `setRaw(key, raw)`, `commit(key, parsed)` (invalid → state invalid, không schedule; hợp lệ → dirty + `schedule(buildPayload())`), `flush()`, `discard()` (= `cancel()` rồi reset raw về server).
- `buildPayload()`: mọi key dirty và hợp lệ → `{student_id, component_id, score | null}` (rỗng = `null` = xoá ô, như grid cũ).
- `dirtyKeys`, `invalidKeys`, `isSaving`, `scoredCount(studentId)`, `avg(studentId)`, `editableStudentIds` (thứ tự roster, chỉ present/late) — Phase 4 dùng để điều hướng.
- Lưu qua `useSaveSessionScores(sessionId)`; onSuccess: xoá dirty của **các key có trong payload vừa gửi** (không xoá key được sửa trong lúc bay), đặt `savedKeys` tạm để flash; toast một lần "Đã lưu điểm thành phần (n ô) — buổi X" (copy giữ). Lưu ý TanStack `mutate` **không xếp hàng**: gọi khi đang bay sẽ chạy song song, nên guard `isPending` là bắt buộc.
- Điều kiện chấm: `editable = held && canWrite && (status === "present" || status === "late")`.

`useMediaQuery(query)` bằng `useSyncExternalStore` với `window.matchMedia`.
**Query cho mobile là `(max-width: 639px)`**, không phải phủ định của `min-width`:
`src/test/setup.ts:32-44` stub `matchMedia` trả `matches:false` cho mọi query, nên
`(max-width: 639px)` → `false` → desktop, đúng mặc định jsdom; dùng `!useMediaQuery("(min-width…")`
sẽ làm mọi test hiện có chạy ở chế độ mobile. Test mobile override
`vi.spyOn(window, "matchMedia")` trả `matches: query.includes("max-width")` và
`afterEach(() => vi.restoreAllMocks())`.

## Related Code Files

| Action | File | Ghi chú |
|--------|------|---------|
| Create | `apps/web/src/lib/hooks/use-media-query.ts` | `useMediaQuery(q): boolean` |
| Create | `apps/web/src/lib/hooks/__tests__/use-media-query.test.ts` | mock matchMedia, đổi `matches` và gọi listener |
| Create | `apps/web/src/features/teaching/hooks/use-score-draft.ts` | |
| Create | `apps/web/src/features/teaching/lib/score-entry-summary.ts` | `summarize(cells, rosterRows, components)` → `{scoredStudents, total, dirtyCount, perStudent}` |
| Create | `apps/web/src/features/teaching/components/score-entry-by-student.tsx` | |
| Create | `apps/web/src/features/teaching/components/score-entry-footer.tsx` | props `scoredStudents,total,dirtyCount,invalidCount,isSaving,onSave` |
| Create | `apps/web/src/features/teaching/components/unsaved-scores-guard.tsx` | bọc `HvConfirmDialog` với copy chuẩn |
| Modify | `apps/web/src/features/teaching/components/session-detail-panel.tsx` | thay `ComponentScoreGrid` → `ScoreEntryByStudent`; `onClose`/đổi tab qua guard; `forwardRef` expose `{flush, discard}`; prop `onDirtyChange` |
| Modify | `apps/web/src/features/teaching/pages/classbook-page.tsx` | `const isMobile = useMediaQuery("(max-width: 639px)")`; mobile → `HvModal` bọc panel; `pendingSessionId` + guard khi đổi buổi |
| Delete | `apps/web/src/features/teaching/components/component-score-grid.tsx` | |
| Delete | `apps/web/src/features/teaching/components/save-button-styles.ts` | caller cuối (grid) biến mất; panel đã bỏ ở Phase 2 |
| Modify | `apps/web/src/features/teaching/lib/classbook-stats.ts` | xoá `parseScoreInput` cũ (dòng ~243) — caller cuối là grid |
| Modify | `apps/web/src/features/teaching/__tests__/classbook-stats.test.ts` | chuyển các case `parseScoreInput` sang `components/hv/__tests__/score-input-parse.test.ts` (đã có ở Phase 1; chỉ bổ sung case còn thiếu) |
| Modify | `apps/web/src/features/roster/__tests__/roster-handlers.ts` | fixture attendance hiện chỉ seed `present`/`absent`/`null` (dòng ~590); thêm học sinh `late` và `excused` cho `session-05` để test D2 và nhóm Vắng. Cross-feature, chỉ đụng file test |
| Create | `apps/web/src/features/teaching/__tests__/use-score-draft.test.tsx` | hook test với msw, **file riêng** với `vi.useFakeTimers()` đầy đủ |
| Create | `apps/web/src/features/teaching/__tests__/score-entry-summary.test.ts` | |
| Rename+Modify | `apps/web/src/features/teaching/__tests__/component-score-grid.test.tsx` → `score-entry-by-student.test.tsx` | giữ fixture (CLASS_ID, comp-mieng/comp-15p, session-05); viết lại case theo UI mới; đổi `toHaveValue(8)` → `toHaveValue("8")` và `toHaveValue(null)` → `toHaveValue("")` (dòng ~114, ~139) vì input giờ là `type="text"` |
| Modify | `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx` | thêm case mobile (spy matchMedia) → panel trong `role="dialog"`; case đổi buổi khi dirty → guard |

## Implementation Steps

1. `use-media-query.ts` + test.
2. `score-entry-summary.ts` (hàm thuần) + test bảng.
3. `use-score-draft.ts`: chuyển logic từ `component-score-grid.tsx` (draft, payload null/score, mutate) sang hook; thêm `invalidKeys`, `savedKeys`, `buildPayload`, debounce với snapshot, `discard` = `cancel` + reset, guard `isPending`.
4. Hook test (`use-score-draft.test.tsx`): `vi.useFakeTimers()` **đầy đủ** (không phải `{toFake:["Date"]}` như test grid cũ ở dòng 49) và `userEvent.setup({ advanceTimers: vi.advanceTimersByTime })`; msw handler PUT đếm số lần gọi và ghi body.
5. `score-entry-by-student.tsx`: accordion `<ul>` với `<li>` hàng (`React.memo`); nút hàng là `<button aria-expanded>`; vùng mở rộng là `<div role="group" aria-label="Điểm của {tên}">` chứa các `HvScoreInput` label `"{component} {tên}"` (giữ pattern label cũ "Điểm 15 phút Nguyễn Văn An"). Nhóm vắng `<details>` với `<summary>Vắng (n)</summary>`, điểm cũ hiển thị text. Footer `sticky bottom-0`.
6. Điều hướng: refs `Map<cellKey, HTMLInputElement>`; `onNavigate` tính key kế theo `[studentIdx][componentIdx]` trên `editableStudentIds`; khi sang học sinh khác thì `setOpenStudent` rồi focus sau `requestAnimationFrame`.
7. Guard: `unsaved-scores-guard.tsx`; `session-detail-panel.tsx` giữ `pendingAction` (close | tab) và mở guard khi dirty; expose `{flush, discard}` qua `useImperativeHandle`; gọi `onDirtyChange`.
8. `classbook-page.tsx`: `pendingSessionId` + guard khi đổi buổi; mobile bottom sheet; desktop giữ layout.
9. Xoá grid, `save-button-styles.ts`, `parseScoreInput` cũ; chuyển test parse; sửa roster fixture; viết lại test; chạy `npx vitest run src/features/teaching src/features/roster src/lib/hooks` + lint/typecheck.
10. Kiểm tra tay: 1080px với bộ 8 cột/20 học sinh; 390px (bottom sheet); bàn phím Enter/Shift+Enter.

## Test scenarios

| Case | Assertion |
|------|-----------|
| Buổi held, học sinh present/late/absent/excused | 2 hàng chấm được (late có ô nhập), nhóm "Vắng (2)" ở cuối không có input |
| Học sinh absent có điểm lưu sẵn (fixture) | trong nhóm Vắng thấy text điểm, không có input, không PUT khi lưu |
| Mở hàng, gõ "7,5" ở "Điểm 15 phút An", blur | ô `data-state="dirty"`; footer "1 ô chưa lưu"; sau 800ms PUT với `7.5`; footer "0 ô chưa lưu"; toast một lần |
| Gõ hai ô (An/Miệng, An/15 phút) trong 800ms rồi chờ | **một** PUT chứa cả hai entry |
| Bấm "Lưu điểm" ngay sau khi gõ | PUT ngay, không có PUT thứ hai sau 800ms |
| Gõ, "Bỏ thay đổi", unmount panel | **không** PUT (cancel trước discard) |
| Blur khi mutation đang bay (delay handler) | không PUT thứ hai cho tới khi PUT đầu xong; sau đó PUT với ô còn dirty |
| Xoá nội dung ô đã có điểm | payload `score: null` |
| Gõ "abc" | `aria-invalid`, nút Lưu disabled, không PUT sau 800ms |
| Enter ở ô cuối hàng 1 | hàng 2 mở, focus ô đầu hàng 2 |
| Đóng panel khi dirty | dialog "Còn 1 ô chưa lưu"; "Lưu và đóng" → PUT rồi `onClose` |
| Chọn buổi khác khi dirty (classbook-page.test) | guard hiện; "Bỏ thay đổi" → panel buổi mới, không PUT |
| `canWrite=false` | không có input, chỉ text điểm |
| Buổi planned (chưa held) | copy "Chấm điểm sau khi buổi diễn ra", không input |
| Mobile (spy matchMedia, `afterEach` restore) | panel trong `role="dialog"`, nút đóng aria-label |
| Lỗi PUT | toast danger hiện có; ô vẫn dirty |

## Success Criteria

- [x] Không còn `overflow-x-auto` trong tab Điểm buổi; không import `component-score-grid`; `save-button-styles.ts` và `parseScoreInput` trong `classbook-stats.ts` đã xoá.
- [x] Bảng test trên xanh; `classbook-page.test.tsx` cũ vẫn xanh ở chế độ desktop mặc định.
- [x] Tab Điểm buổi ở 1080px với 8 cột không cuộn ngang; bottom sheet ở 390px dùng được. — *đã kiểm với bộ **10 cột**: `scores-panel-1080.png` (lưới 3 ô/hàng, không cuộn ngang), `scores-sheet-390.png` (lưới 2 ô/hàng trong sheet) trong `plans/reports/screenshots-260902-scoring-ui/`; guard invalid/dirty chụp ở `guard-invalid-1280.png`, `guard-dirty-1280.png`.*
- [x] Không PUT trùng, không PUT rơi ô, không PUT sau discard (ba test fake timers).

## Risk Assessment

- **Debounce + mutation đang bay**: `isPending` hoãn `schedule`; `onSettled` còn dirty thì `schedule` lại. Test bắt buộc.
- **`schedule` thay thế payload** (`use-debounced-save.ts`): luôn snapshot toàn bộ dirty set; test hai ô trong 800ms bảo vệ.
- **Flush khi unmount** (`use-debounced-save.ts:60`): `discard()` gọi `cancel()`; test discard→unmount bảo vệ.
- **Server echo ghi đè ô đang gõ**: `useSaveSessionScores` thay cache toàn bộ; `useScoreDraft` chỉ đồng bộ `server`, không đụng `raw` của key còn dirty.
- **Accordion một hàng làm chậm nhập liệu 20 học sinh**: Enter ở ô cuối tự mở hàng kế; Phase 4 cung cấp bảng đầy đủ.
- **Test cũ phụ thuộc label ô**: giữ format label `"Điểm {cột} {tên}"`.
