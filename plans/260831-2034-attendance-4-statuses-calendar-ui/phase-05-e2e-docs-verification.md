---
phase: 5
title: "E2E, docs, verification"
status: completed
priority: P2
effort: "0.5d"
dependencies: [3, 4]
---

# Phase 5: E2E, docs, verification

## Overview

Khép vòng: cập nhật e2e Playwright theo flow mới, gỡ dần body legacy trong
spec, regen swagger, cập nhật docs, đối chiếu kết quả với acceptance criteria
của plan.

## Requirements

- [x] E2E attendance: flow 4 trạng thái end-to-end (chọn lớp → 3 thẻ → đánh muộn/vắng/lý do → xác nhận → mở lại sửa); chạy trên stack `teka-e2e` (statement specs cần fresh seed).
- [x] E2E chuyển buổi bằng ‹ › và lịch tháng.
- [x] Swagger (`make api-docs`) đã regen trong Phase 1–2 — verify không còn diff treo.
- [x] Docs: cập nhật phần attendance/session trong docs hiện có (`docs/api-guidelines.md` chỉ khi contract convention đổi; tìm doc mô tả nghiệp vụ điểm danh qua README/docs nav thay vì giả định tên file).
- [x] Chạy full: `make test-api`, `npm run typecheck && npm run test`, `npm run e2e`.

## Implementation Steps

1. Cập nhật/viết e2e specs; xác nhận seed đủ dữ liệu 4 trạng thái.
2. Rà docs nav từ README → cập nhật surface nhỏ nhất sở hữu nghiệp vụ điểm danh.
3. Chạy toàn bộ gates; sửa regression thay vì nới test.
4. Đối chiếu Success Criteria của plan.md, đánh dấu hoàn thành từng mục.

## Success Criteria

- [x] Mọi gate xanh trên stack e2e cô lập.
- [x] Không còn TODO/legacy body trong e2e specs (trừ 1 test giữ chủ đích cho tương thích ngược nếu quyết định giữ).

## Risk Assessment

- **E2E flaky do dữ liệu dùng chung** — dùng compose project `teka-e2e` với seed
  mới, 1 worker như quy ước hiện tại.
- **Docs drift** — chỉ cập nhật doc sở hữu nhỏ nhất, link tới swagger thay vì
  chép chi tiết contract.
