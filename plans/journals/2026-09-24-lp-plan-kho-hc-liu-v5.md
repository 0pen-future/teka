---
title: Lập plan Kho học liệu v5
date: 2026-09-24
summary: "Plan 6 phase đưa menu Kho học liệu lên prototype v5 (ba ngân hàng, 7 tab chi tiết, nhóm bài tập, nhiều bộ điểm); validation gate chốt 6 quyết định theo mặc định"
---

# Lập plan Kho học liệu v5

## What happened
- Import prototype `So Lop - Prototype v5` từ claude.ai/design (màn `lib`, `td`, `ts`) và đối chiếu với hiện trạng feature `library` (API Go + web React). Scout report: `plans/reports/scout-260924-1140-library-current-state.md`.
- Viết `plans/260924-0448-kho-hoc-lieu-v5/` (plan.md + 6 phase): API ngân hàng (migration 000033: kind 7 loại, `active`, mã bài tập tự sinh, skill/level), API phiên bản (000034: `mode`/`unit`, `template_exercise_groups`, `score_set` thành mảng bộ, 2 loại nhật ký mới, đếm lớp), web hub 3 tab, chi tiết 7 tab, trang buổi học 2 tab, e2e/seed/docs/ship. Ước tính 10.5d.
- Verification pass 12 claim đối chiếu source (constraint inline ở 000028, `copyVersionContent` không copy prep, `MigrateDown(m, 28)` cứng, `useSearch` cục bộ trong `items-tabs.tsx`, `make seed` là lệnh seed chuẩn) → sửa 3 chi tiết trong phase 2/3/6.

## Decision
- Không thêm permission key; published vẫn bất biến (409 `VERSION_LOCKED`); ngân hàng dùng `active` flag thay xoá cứng; `level` text mới, giữ `difficulty`; reshape `score_set` JSONB (tách biệt `grading.score_sets`); `classprogram.Apply` vẫn copy mọi buổi; upload file và nhập buổi từ file là non-goal.
- `ak plan add-phase` bỏ dấu tiếng Việt khỏi slug → đổi tên file ASCII rồi `ak plan reindex`.

## Next steps
- `/ak:cook plans/260924-0448-kho-hoc-lieu-v5/plan.md` sau khi Phase 9 của plan `260923-0715-giang-day-menu` ship.
- Backup DB trước khi chạy migration 000034 (reshape JSONB).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
