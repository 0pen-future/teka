---
phase: 5
title: "Web score entry (classbook session panel)"
status: done
priority: P1
effort: "1d"
dependencies: [3]
---

# Phase 5: Web score entry (classbook session panel)

<!-- Updated: Validation Session 1 - UI thay thế (không sống cạnh); step 0.5 -->


## Overview

Nhập điểm thành phần trong tab "scores" của
`session-detail-panel.tsx` (feature `teaching`): lớp có
`class_score_components` ⇒ grid điểm thành phần render **thay cho** input
điểm chung; lớp không có ⇒ zero change (user chốt tại validation 260829).

## Requirements

- Functional:
  - Lớp không có components: panel giữ nguyên như hiện tại (zero change).
  - Lớp có components: grid học sinh × thành phần **thay thế** block điểm
    chung trong tab scores. Input 0–10 `step={0.5}` (mirror input điểm
    chung hiện tại — user chốt 0.5, không nâng 0.1; API/DB vẫn nhận 1 chữ
    số thập phân), chỉ editable khi buổi `held` và học sinh `present`
    (mirror rule, `session-detail-panel.tsx:279`).
  - Nút "Lưu điểm thành phần" + dirty indicator theo pattern "Chưa lưu"
    hiện có; batch PUT `/sessions/:id/scores`; xóa = clear input (gửi
    null).
  - Owner xem classbook lớp giáo viên khác cũng nhập được (AC4).
  - SessionMark model/API/hook giữ nguyên — chỉ ẩn lối nhập điểm chung ở
    lớp dùng components; điểm chung cũ (nếu có) vẫn hiển thị ở
    chart/records như trước.
- Non-functional: helper text ghi rõ "điểm thành phần chưa vào báo cáo phụ
  huynh" (lớp dùng components không sinh điểm chung mới ⇒ chart/báo cáo
  Zalo không nhận thêm dữ liệu — trade-off user đã chấp nhận, follow-up
  đưa vào báo cáo); grid cuộn ngang trong container khi >3 components
  (mobile), soi cả chiều dọc (panel đã có vùng cuộn `max-h-[280px]`).

## Architecture

- Decision 5 (plan.md, user chốt): **thay thế** — render có điều kiện
  trong tab scores: `components.length > 0` ⇒ grid thành phần, ngược lại
  block điểm chung như cũ. SessionMark không đổi ở model/API/chart.
- Feature `teaching` mở rộng:
  - `api/` thêm client GET/PUT scores + GET class score-components (soi
    file api hiện có của teaching để đặt đúng chỗ).
  - `hooks/use-component-scores.ts` — query theo (classId, sessionId) +
    mutation invalidate đúng key.
  - `components/component-score-grid.tsx` — grid tách riêng, panel chỉ
    compose (panel đã 339 dòng, không phình thêm inline).
- Draft state cục bộ trong grid (pattern `scoreDraft` hiện có), parse qua
  `parseScoreInput` tái dùng từ `../lib/classbook-stats`.

## Related Code Files

- Modify: `apps/web/src/features/teaching/components/session-detail-panel.tsx`
- Create: `apps/web/src/features/teaching/components/component-score-grid.tsx`
- Create: `apps/web/src/features/teaching/hooks/use-component-scores.ts`
- Modify: `apps/web/src/features/teaching/api/*` + `schemas/*` (thêm client/zod)
- Create/Modify: `apps/web/src/features/teaching/__tests__/` (test grid +
  panel có/không components; MSW handlers mới)

## Implementation Steps

1. Soi `use-class-marks` + `use-teaching-mutations` để khớp query-key và
   invalidation hiện có.
2. API client + zod schemas + hook.
3. `component-score-grid.tsx`: cột theo position, row theo roster; editable
   rule mirror điểm chung; dirty tracking + nút lưu riêng.
4. Compose vào tab scores: `components.length > 0` ⇒ grid thay block điểm
   chung; ngược lại giữ block cũ nguyên vẹn.
5. Tests: không components → panel không đổi (snapshot/queries); có
   components → block điểm chung ẩn, grid nhập/lưu, PUT payload đúng;
   absent/chưa held → readonly.
6. `npm run typecheck && npm run test`; smoke e2e nếu stack đang chạy
   (không bắt buộc cho phase này).

## Success Criteria

- [ ] AC4 phía UI: teacher + owner nhập/sửa từng học sinh × thành phần ×
      buổi.
- [ ] Lớp không có components: không thay đổi hành vi/giao diện.
- [ ] typecheck + vitest pass, không regress test teaching hiện có.

## Risk Assessment

- Grid rộng với nhiều components trên mobile → overflow-x trong container
  (requirement non-functional), không đổi layout panel.
- **Chart/báo cáo không nhận dữ liệu mới từ lớp dùng components** (điểm
  chung không còn lối nhập ở đó): trade-off user đã chấp nhận tại
  validation; helper text làm kỳ vọng rõ ràng. Đưa điểm thành phần vào
  chart/báo cáo phụ huynh là follow-up gần như chắc chắn.
- Lớp bị gỡ snapshot (clear khi chưa có điểm) quay về block điểm chung —
  render có điều kiện phải phản ứng theo query components, không cache
  cứng.
