---
title: "Giảng dạy phase 2: lời mời nhận lớp và đội ngũ giảng dạy"
date: 2026-09-23
summary: "Hoàn thành phase 2 plan Giảng dạy: API class_invitations, trang /class-invitations, ClassTeamSection; review SHIP WITH FIXES đã xử lý"
---

# Giảng dạy phase 2: lời mời nhận lớp và đội ngũ giảng dạy

## What happened
- Backend: package `classinvites` (state machine pending → accepted/declined → assigned/cancelled), migration `000026_class_invitations`, 7 route trong routespec, hook hủy lời mời khi `RemoveMember`, swagger sinh lại bằng `make api-docs`.
- Web: trang `/class-invitations` (chip lọc, hành động theo vai), `ClassTeamSection` thay placeholder trong tab Thông tin, dialog mời và dialog xác nhận bàn giao, nav + overflow, MSW fixture, vitest, e2e `class-invitations.spec.ts`.
- Review bởi code-reviewer: SHIP WITH FIXES, không có Critical/High. Đã sửa M1/M2/M3/L1/L3/L5/L6 và nit message rỗng; giữ L2/L4 có lý do. Báo cáo: `plans/260923-0715-giang-day-menu/reports/review-phase-02-260923.md`.

## Lessons / evidence
- Pre-check khi gửi lời mời phải phản chiếu invariant `uq_class_staff_active` (một stint active mỗi người mỗi lớp), nếu không sẽ tạo lời mời không bao giờ confirm được. Sửa bằng `refuseHeldRole`, unit test phủ ba nhánh.
- Confirm `giao_vien` là bàn giao lớp nên phải invalidate `classesKeys.all` và `sessionsKeys.all` như `useReassignTeacher`; assert bằng `vi.spyOn(queryClient, "invalidateQueries")` vì query buổi học không mount trong test.
- Trong vitest, `server.resetHandlers()` giữa chừng xoá cả handler roster của test; dùng `server.use(handler, { once: true })` cho fixture lỗi tạm thời.
- E2E file dưới `tsconfig.node.json` phải import helper với hậu tố `.js`, nếu không eslint báo `no-unsafe-call`.
- Gate: `go test ./internal/features/classinvites/` ok; integration `-p 1` classinvites + centers ok; `make test-api-unit`, `make scopelint`, `make lint` xanh; vitest roster 190 passed; e2e cô lập chạy hai lần (trước và sau fix) đều 1 passed, không để lại container/volume `teka-e2e`.

## Decision
- Thành viên đã rời trung tâm accept/decline trả 404 (theo verification (6) của phase file), thay vì 409 như code ban đầu.
- Không phân trang `GET /class-invitations` (đồng dạng với `GET /classes/:id/staff`); ghi nhận xem lại ở Phase 9 nếu dòng terminal tăng.
- Không chặn Send vào lớp `archived`: spec không yêu cầu, để user quyết.

## Next steps
- Commit `feat(api)`, `feat(web)`, `chore(plans)` cho phase 2.
- Re-scout rồi thực hiện Phase 3.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
