---
phase: 1
title: "Dữ liệu theo tháng và trạng thái buổi"
status: pending
priority: P1
effort: "0.5d"
dependencies: []
---

# Phase 1: Dữ liệu theo tháng và trạng thái buổi

## Overview
Cho phép trang đọc buổi/nhận xét/điểm theo tháng chọn từ URL (`?month=YYYY-MM`)
và tính sẵn trạng thái nhận xét, số học sinh đã chấm cho từng buổi để bảng và
dải KPI chỉ còn việc hiển thị. Không có UI trong phase này.

## Requirements
- Functional:
  - `monthWindow(month: string, today: string)` → `{ from, to, label, month }`:
    `from` = mùng 1; `to` = hôm nay nếu `month` là tháng hiện tại, ngược lại là
    ngày cuối tháng; `label` = "MM" (giữ tên file CSV cũ), `month` = "YYYY-MM".
  - `parseMonthParam(raw: string | null, today: string)` → "YYYY-MM" hợp lệ,
    fallback tháng hiện tại khi thiếu/sai định dạng.
  - `shiftMonth(month, delta)` cho stepper.
  - `useMonthSessions(classId, month)` nhận tháng, trả thêm `month` (cửa sổ
    trên) và `isError`; giữ `useQueries` roster như cũ.
  - `sessionWorkStatus(derived, sessionNote, scoredCount)` → `{ hasNote,
    scored, total, noteChip: "done" | "missing" | "none", scoreChip:
    "done" | "partial" | "none" }` (`none` cho buổi hủy/dự kiến; `total` =
    `present` của buổi).
  - `useSessionScoreCounts(heldSessionIds, enabled)` trong
    `hooks/use-session-score-counts.ts`: `useQueries` với
    `teachingKeys.sessionScores(id)` + `getSessionScores`, trả
    `Record<sessionId, number>` = số học sinh có ≥1 ô điểm. Chỉ `enabled` khi
    lớp có bộ điểm.
- Non-functional: thuần hàm, test đơn vị trong `classbook-stats.test.ts`;
  không đổi query key hiện có để cache panel/bảng dùng chung.

## Architecture
- `lib/classbook-stats.ts` là nơi đặt `monthWindow`, `parseMonthParam`,
  `shiftMonth`, `sessionWorkStatus`, `scoredStudentCount(sessionScores[id])`.
  Kiểm tra `monthRange` đang có (dùng cho retention) và tái dùng phần tính
  ngày đầu/cuối thay vì viết trùng.
- `hooks/use-month-sessions.ts`: bỏ import `currentMonth` từ roster; nhận
  `month` (YYYY-MM) và `today` (ISO, mặc định `new Date()`), gọi
  `useSessionsList(classId, { from, to })`.
- Chip CHẤM ĐIỂM: nguồn đếm chọn ở page: `hasScoreComponents ?
  componentCounts[id] : scoredStudentCount(sessionScores[id])`.

## Related Code Files
- Modify: `apps/web/src/features/teaching/lib/classbook-stats.ts`
- Modify: `apps/web/src/features/teaching/hooks/use-month-sessions.ts`
- Create: `apps/web/src/features/teaching/hooks/use-session-score-counts.ts`
- Modify: `apps/web/src/features/teaching/__tests__/classbook-stats.test.ts`
- Read: `apps/web/src/features/roster/lib/current-month.ts`,
  `apps/web/src/features/teaching/hooks/use-component-scores.ts`,
  `apps/web/src/features/teaching/api/teaching-api.ts` (`getSessionScores`),
  `apps/web/src/features/teaching/schemas/teaching-schemas.ts` (`SessionScoresResponse`).

## Implementation Steps
1. Đọc `classbook-stats.ts` (`monthRange`, `retentionStat`) và
   `current-month.ts`; viết `monthWindow`, `parseMonthParam`, `shiftMonth`
   dùng chuỗi ISO cục bộ (không `toISOString` để tránh lệch múi giờ).
2. Viết `scoredStudentCount(scores?: Record<string, number>)` và
   `sessionWorkStatus(...)`; buổi `cancelled`/`planned` → `none`.
3. Sửa `useMonthSessions(classId, month)`; cập nhật chỗ gọi tạm trong page
   (`parseMonthParam(searchParams.get("month"))`) để typecheck xanh.
4. Viết `useSessionScoreCounts`; đếm học sinh có ≥1 entry trong
   `data.scores` theo `student_id`.
5. Test: `monthWindow` cho tháng hiện tại / quá khứ / tương lai, `parseMonthParam`
   với null, "2026-13", "abc"; `shiftMonth` qua năm; `sessionWorkStatus` cho
   held có note + đủ điểm, held thiếu, cancelled, planned.

## Success Criteria
- [x] `npm run test -- classbook-stats` xanh với các case mới.
- [x] `npm run typecheck` xanh sau khi đổi chữ ký `useMonthSessions`.
- [x] Test classbook-page hiện có vẫn xanh (tháng mặc định = tháng hiện tại,
      `to` = hôm nay).

## Risk Assessment
- Lệch ngày do múi giờ khi tính cuối tháng → dùng `new Date(y, m, 0)` cục bộ
  và format thủ công như `current-month.ts`.
- `getSessionScores` cho buổi chưa có điểm trả `scores: []` → đếm 0, không lỗi.
