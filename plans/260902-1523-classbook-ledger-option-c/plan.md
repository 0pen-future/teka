---
title: "Sổ lớp mở rộng tại chỗ — Quản lý lớp học theo Phương án C"
description: "Dựng lại trang /classbook (ClassbookPage) đúng 100% Phương án C trong report ui-redesign-260902-1454: toolbar chọn lớp + month stepper, dải KPI hairline, một bảng sổ lớp 8 cột với chip trạng thái, hàng mở rộng 3 khối tại chỗ thay cho panel/sheet; web-only, không API mới."
status: completed
priority: P1
effort: "3-4d"
tags: [web, ui, hv-kit, teaching, classbook]
created: 2026-09-02
blockedBy: []
blocks: []
---

# Sổ lớp mở rộng tại chỗ — Quản lý lớp học theo Phương án C

## Overview

Nguồn thiết kế: `plans/reports/ui-redesign-260902-1454-classbook-page.html`,
mục **Phương án C · Sổ lớp mở rộng tại chỗ** (`#opt-c`) và mục
**5 · Áp dụng cho mọi phương án** (`#chung`). Plan này phải đạt **100% UI như
Phương án C** mô tả, gồm cả stage desktop 1280 và stage mobile 390.

Trang hiện tại (`apps/web/src/features/teaching/pages/classbook-page.tsx`)
gồm: header + nút CSV, tablist pill chọn lớp, 5 thẻ KPI (`ClassStatCards`),
tab gạch chân Buổi học / Chương trình, bảng grid 6 cột (`SessionsTable`) với
`SessionDetailPanel` dạng card bên phải (desktop) hoặc `HvModal` sheet
(mobile, qua `useMediaQuery`). Phương án C thay toàn bộ lớp trình bày và bỏ
nhánh mobile riêng, nhưng **giữ nguyên hooks dữ liệu, mutation, guard điểm chưa
lưu và các component chấm điểm** (`ScoreEntryByStudent`, `ScoreTableModal`,
`UnsavedScoresGuard`, `PlanSummary`, `PlanStatusPill`).

**Phạm vi web-only.** Không thêm endpoint; mọi trạng thái nhận xét / chấm
điểm tính từ dữ liệu đã có (`deriveSessions`, `useClassMarks`,
`useSessionScores`).

### Outcome, constraints, non-goals, acceptance

- **Outcome:** `/classbook?class_id=…&month=YYYY-MM` hiển thị đúng bố cục C:
  toolbar (h1, nút chọn lớp có tìm kiếm, month stepper, HvSegmented Buổi học |
  Chương trình & giáo án, nút CSV ghost icon) → dải KPI 4 ô trên hairline →
  bảng 8 cột BUỔI / BÀI HỌC / GIÁO ÁN / CÓ MẶT / ĐTB / DOANH THU / NHẬN XÉT /
  CHẤM ĐIỂM → hàng mở rộng 3 khối (Nhận xét chung, Giáo án, Điểm buổi) + footer
  (gợi ý phím, "n ô chưa lưu", Đóng). Mobile 390: cùng bảng, cùng hàng mở
  rộng, ẩn cột, 3 khối xếp dọc.
- **Constraints:** chỉ dùng token/primitive sẵn có (`@/components/hv`, Tailwind
  token cream/mint/sky/sun/coral/ink/line, radius/shadow đã khai báo); import
  cross-feature qua `index.ts`; không sửa `src/components/ui/`; test Vitest +
  MSW offline; `npm run test`, `npm run typecheck`, `npm run lint` xanh.
- **Non-goals:** không đổi API/DB; không sửa tab Chương trình & giáo án
  (`CourseView`) ngoài việc gắn vào HvSegmented; không đổi trang Hồ sơ học sinh
  (`student-record-page.tsx`, `StudentSessionsTable`); không làm Phương án A/B
  hay ghép B+C; không đưa KPI strip / chip lên `components/hv` (chưa có nơi tái
  dùng thứ hai).
- **Acceptance:** xem mục Success Criteria bên dưới; mọi ý trong bảng
  "Việc phải làm" của Phương án C được thực hiện (Mới: `class-select.tsx`,
  `class-kpi-strip.tsx`, `session-expand-row.tsx`; Sửa: `sessions-table.tsx`,
  `session-detail-panel.tsx` → 3 khối, `classbook-page.tsx` bỏ `isMobile` +
  `HvModal`; Xóa: `class-stat-cards.tsx`, nhánh sheet).

## Research notes (đã xác minh trong repo)

- `useMonthSessions(classId)` (`hooks/use-month-sessions.ts`) khóa cứng
  `currentMonth()` từ `@/features/roster` (`from` = mùng 1, `to` = hôm nay,
  `label` = "MM"). Month stepper cần tham số tháng → thêm helper cửa sổ tháng
  trong `lib/classbook-stats.ts` (đã có `monthRange` cho retention, kiểm tra
  tái dùng) và truyền `month` vào hook.
- `useClassMarks(classId, "YYYY-MM")` trả `sessionNotes[sessionId].text`,
  `sessionScores[sessionId][studentId]` (điểm chung). Điểm thành phần nằm ở
  `useSessionScores(sessionId)` (per-session, key
  `teachingKeys.sessionScores`). Chip CHẤM ĐIỂM n/N: khi lớp **không** có bộ
  điểm → đếm học sinh có điểm chung trong `sessionScores`; khi lớp **có** bộ
  điểm → `useQueries` theo buổi đã dạy (cùng pattern `useQueries` roster đang
  dùng trong `useMonthSessions`), đếm học sinh có ≥1 ô điểm.
- `SessionDerived` đã có `present`, `eligible`, `average`, `gross`, `net`,
  `lessonIndex`; `ClassbookTotals` đã có mọi số cho KPI (attendancePct,
  presentTotal/eligibleTotal, classAverage, scoredSessionCount, monthGross,
  monthCost, monthNet); `retentionStat` cho dòng phụ "tái tục %".
- Lớp (`classSchema`) **không có tên giáo viên** (chỉ `teacher_id`) và không
  có số học sinh. Nút chọn lớp hiển thị `name` + `"{sĩ số} HS · {lịch}"` với sĩ
  số từ `activeHeadcount(enrollments)` của lớp đang chọn và lịch từ
  `formatScheduleSummary(schedules, today)` (đã export ở roster). "Cô Lan"
  trong mock không có nguồn dữ liệu → bỏ, ghi nhận là sai lệch có chủ ý.
- `SessionDetailPanel` hiện gom cả 3 tab, note draft, general-score draft,
  guard nội bộ (đổi tab/đóng) và handle `flush/discard/requestClose`. Với 3
  khối hiện đồng thời, guard đổi tab không còn; guard đóng/đổi dòng vẫn cần
  và đã có ở page (`UnsavedScoresGuard` + `pendingSelection`).
- `HvSegmented` có `variant="tabs"` (tablist/tab, panel id
  `{idBase}-panel-{value}`) và `variant="segmented"` (radio group). Mock C dùng
  segmented cho Buổi học | Chương trình & giáo án → dùng `variant="tabs"` để giữ
  a11y tablist như tab hiện tại.
- `HvStateBlock` (`loading | empty | error`, `compact`, `action`) dùng cho
  đang tải / trống / lỗi / chưa có lớp. `ProgressBar` cho mini bar CÓ MẶT.
  `HvIcon` có `chevron-down`, `file`, `table`, `arrow-up/down`, `x`; thiếu
  `chevron-left/right` cho stepper → thêm vào registry `hv-icon.tsx` (Lucide
  `ChevronLeft`, `ChevronRight`), đây là mở rộng registry hợp lệ.
- `useMediaQuery` chỉ còn `classbook-page.tsx` dùng (+ test riêng của hook).
  Sau khi bỏ khỏi page, xóa hook và test của nó (YAGNI); mock `matchMedia`
  trong test "opens the detail panel as a sheet" bị xóa theo.
- Test hiện có `__tests__/classbook-page.test.tsx` (frozen clock 2026-08-20,
  lớp Toán 6A, buổi 05/08 held, 08/08 cancelled "Nghỉ lễ", 12/08 held, 19/08
  planned, 26/08 ngoài cửa sổ) và `classbook-course.test.tsx` cần cập nhật
  selector: `statCard(...)` → KPI strip, `findHeldRow` từ `button` → `row`
  có `aria-expanded`, tab "Điểm buổi" → khối luôn hiện, heading panel → nhãn
  hàng mở rộng, sheet test → test cột ẩn/khối xếp dọc không cần `matchMedia`.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Month stepper + `?month=YYYY-MM`, dữ liệu buổi/nhận xét/điểm theo tháng đã chọn | P1 |
| 2 | Toolbar C: chọn lớp có tìm kiếm, HvSegmented đổi view, CSV ghost icon, dải KPI 4 ô | P1 |
| 3 | Bảng sổ lớp 8 cột với chip trạng thái, hàng hủy/dự kiến, responsive ẩn cột + cột VIỆC mobile | P1 |
| 4 | Hàng mở rộng 3 khối tại chỗ + footer, một hàng mở, guard, phím ↑ ↓ Enter, bỏ HvModal/isMobile | P1 |
| 5 | Test cập nhật, xóa code chết, docs, lint/typecheck xanh | P1 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Dữ liệu theo tháng và trạng thái buổi](./phase-01-month-data-and-status.md) | Pending |
| 2 | [Phase 2: Toolbar, chọn lớp, month stepper, dải KPI](./phase-02-toolbar-and-kpi-strip.md) | Pending |
| 3 | [Phase 3: Bảng sổ lớp 8 cột với chip trạng thái](./phase-03-ledger-table.md) | Pending |
| 4 | [Phase 4: Hàng mở rộng 3 khối và điều hướng](./phase-04-expand-row.md) | Pending |
| 5 | [Phase 5: Test, dọn dẹp, docs, verify](./phase-05-tests-cleanup-docs.md) | Pending |

Thứ tự: 1 → 2 → 3 → 4 → 5. Phase 2 và 3 có thể chạy song song sau Phase 1
(file khác nhau), Phase 4 cần cả 2, Phase 5 sau cùng.

## Red-team (tự phản biện, đã chốt)

| Lo ngại | Quyết định |
|---------|-----------|
| Khối điểm cao với lớp đông (20 HS) đẩy bảng xuống | Khối Điểm buổi trong hàng mở rộng giới hạn `max-h` cuộn trong + "Mở bảng đầy đủ" (đã có `ScoreTableModal`). Đúng như report nêu. |
| 8 cột ở 1024px sát giới hạn | Bảng nằm trong wrapper `overflow-x-auto`, `min-w` cho bảng ở ≥640px; ≤639px ẩn cột theo spec nên không cần cuộn. |
| N+1 query điểm thành phần cho chip CHẤM ĐIỂM | Chỉ bật khi lớp có bộ điểm, chỉ cho buổi `held` trong tháng (≤ ~12 query, cùng cache key panel đang dùng nên không tải lại khi mở hàng). |
| `?month` tương lai/quá khứ: `to` là gì? | Tháng hiện tại → `to` = hôm nay (giữ hành vi cũ, test 26/08 ngoài cửa sổ vẫn đúng); tháng khác → trọn tháng. Stepper không chặn tương lai (buổi `planned`). |
| Bỏ `useMediaQuery` có phá test/hook khác? | Đã grep: chỉ page dùng. Xóa cả hook và test. |
| Đổi `button` hàng → `tr` có phá a11y? | Hàng là `<tr>` với `role="row"` mặc định, ô BUỔI chứa `<button aria-expanded aria-controls>`; toàn hàng click qua `onClick` trên `tr` nhưng tên a11y lấy từ nút. Bàn phím ↑↓ xử lý ở `tbody` `onKeyDown` khi focus nằm trong nút hàng. |
| Guard điểm chưa lưu khi đổi tháng / đổi view | `requestSelection` mở rộng thành `requestNavigation` với 4 loại: `session`, `class`, `month`, `view`; cùng modal `UnsavedScoresGuard`. |
| Tên giáo viên trong nút chọn lớp | Không có dữ liệu → bỏ, không thêm API (non-goal). |

## Success Criteria

- [x] Toolbar: h1 "Quản lý lớp học"; nút chọn lớp (tên đậm + "n HS · lịch" + chevron) mở danh sách có ô tìm (`useClassSearch`, hiện ô tìm khi > 5 lớp) và ghi `class_id`; month stepper "‹ Tháng M/YYYY ›" ghi `?month=YYYY-MM` (replace), mặc định tháng hiện tại; `HvSegmented` "Buổi học | Chương trình & giáo án"; nút CSV ghost icon `aria-label="Tải dữ liệu lớp (CSV)"`. *(Sai lệch: icon dùng `arrow-down` vì hv-icon không có `download`; nút chọn lớp không hiện tên giáo viên — không có dữ liệu, xem bảng quyết định.)*
- [x] Dải KPI hairline 4 ô: SĨ SỐ (sub "tái tục x%"), CHUYÊN CẦN (sub "a/b lượt"), ĐIỂM TB (sub "n buổi"; "—" khi chưa chấm), LÃI/LỖ Tm (sub "thu · chi" một dòng; âm dùng `text-coral-600`); số `tabular-nums`; mobile 2×2.
- [x] Bảng 8 cột đúng thứ tự spec; hàng held: ngày đậm, "Bài n · tiêu đề", `PlanStatusPill`, "x/y" + mini bar, ĐTB, doanh thu, chip Đã có/Chưa có, chip n/N (vàng khi chưa đủ, mint khi đủ); hàng hủy: lý do ở BÀI HỌC, pill "Buổi hủy", các ô còn lại "·" mờ; hàng dự kiến: "14 dự kiến", pill Chờ duyệt/Chưa soạn, ô số "·".
- [x] Hàng đang mở: nền `mint-50`, sub-label "đang mở" dưới ngày, `aria-expanded="true"`; chỉ một hàng mở; hàng mở rộng viền mint trên/dưới, 3 khối cạnh nhau ở ≥900px, xếp dọc dưới đó.
- [x] Khối NHẬN XÉT CHUNG: textarea + "Lưu nhận xét" + trạng thái Chưa lưu/Đã lưu ✓ (read-only khi không có quyền). Khối GIÁO ÁN · BÀI n/N: tiêu đề, `PlanSummary`, `PlanStatusPill`; trống → thông điệp cũ. Khối ĐIỂM BUỔI: `ScoreEntryByStudent` (có bộ điểm) hoặc điểm chung (không bộ điểm); "+ n học sinh"/cuộn trong; "Mở bảng đầy đủ".
- [x] Footer hàng mở rộng: "Di chuyển ↑ ↓ · mở/đóng Enter" (kbd), "n ô chưa lưu" `sun-600` khi dirty, nút "Đóng" ghost.
- [x] Guard `UnsavedScoresGuard` chặn đổi hàng / lớp / tháng / view khi còn ô chưa lưu; "Lưu và đóng" flush rồi tiếp tục; "Bỏ thay đổi" discard.
- [x] Bàn phím: ↑/↓ di chuyển focus giữa các hàng, Enter/Space mở/đóng, Esc đóng hàng đang mở.
- [x] Mobile ≤639px: ẩn GIÁO ÁN, ĐTB, DOANH THU, NHẬN XÉT, CHẤM ĐIỂM; BUỔI hiện "dd/mm" + sub "Bài n"; thêm cột VIỆC (chip Xong / Nhận xét / Chấm điểm / Hủy / Dự kiến); không `HvModal`, không `useMediaQuery`, không `variant="sheet"`.
- [x] Trạng thái: đang tải → `HvStateBlock loading`; tháng trống → `HvStateBlock empty "Chưa có buổi học nào trong tháng m."`; lỗi → `HvStateBlock error`; không có lớp → `HvStateBlock empty "Chưa có lớp đang hoạt động"` + nút "Tạo lớp" (chủ trung tâm, link `/center/classes`).
- [x] CSV vẫn xuất đúng cột và tên file `{lớp}_ky{MM}.csv` theo tháng đang chọn.
- [x] `class-stat-cards.tsx`, `use-media-query.ts` (+ test) bị xóa; `session-detail-panel.tsx` không còn `variant`/tabs. *(Thực tế: file bị xóa hẳn, thay bằng `session-expand-row.tsx`.)*
- [x] `npm run test`, `npm run typecheck`, `npm run lint` trong `apps/web` xanh; test classbook cập nhật selector, không giảm hành vi được phủ. *(Sau code review: thêm test guard điểm chung/nhận xét, Esc trong hàng, `class_id` lạ, chip 0/0, `ProgressBar` aria-label.)*

<!-- slug: classbook-ledger-option-c -->
