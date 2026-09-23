---
title: "Plan --deep menu Giảng dạy: red-team + validation hoàn tất"
date: 2026-09-23
summary: "Kế hoạch 9 phase (15.5d) cho menu Giảng dạy đã qua scout, red-team S1 và validation S1; sẵn sàng /ak:cook."
---

# Plan --deep menu Giảng dạy: red-team + validation hoàn tất

## What happened
- Import prototype `So Lop - Prototype v5.dc.html` từ Claude Design (DesignSync) rồi lập kế hoạch `--deep` tại `plans/260923-0715-giang-day-menu/` (plan.md + 9 phase file, tổng 15.5d).
- Scout repo: feature folder Go, `routespec.Kind`, catalog quyền `CatalogVersion = 4`, khuôn backfill 000022, `handoff`/`classstaff` owner-only, web nav overflow, `RESOURCE_LABELS`.
- Red Team Session 1: 15 cluster áp dụng (proposal model cho lời mời, feature điều phối `classprogram`/`classchat` dựng sau `teaching`, gỡ chương trình không xoá curriculum, `req()` + snapshot test, bump `CatalogVersion` một lần, down migration là hợp đồng, FK composite `center_id`, bỏ bảng `prep_*` song song); 2 từ chối có lý do.
- Validation Session 1 (Q1–Q6): A1 6 mục xác nhận; Phase 8 giữ scope; lời mời = đề xuất + owner xác nhận; key `*.edit`/`prep.assign` opt-in; **áp dụng/đổi/gỡ chương trình lớp chỉ owner** (`KindOwnerOnly`, web gate `isOwner`); Chat giữ Phase 7.

## Lessons / evidence
- `center_members` PK là `(teacher_id, center_id)` (000007:66-72); FK composite trong plan ban đầu viết ngược thứ tự — đã sửa ở phase 2/8.
- Repo dùng `ON DELETE CASCADE` trên FK guard tới `classes` (000009/000015) chỉ như hard-delete guard; lớp soft-delete và thành viên `left_at` không kích hoạt cascade nên logic hủy lời mời phải nằm ở service. D8 tinh chỉnh theo quy ước này.
- `audit_logs` đã có `entity_type`/`entity_id`/`occurred_at` (000010) → index `idx_audit_logs_entity` thay placeholder.
- `GET /classes/:id/sessions` cap 400 ngày (`sessions/service.go:22`) → tab Buổi học cắt cửa sổ `min(end_date, from+399d)`.
- CLI: `ak plan validate` cần **thư mục** plan; `ak plan status`/`phase close` vẫn "Command failed" → giữ trạng thái trong file + `ak plan reindex`.

## Decision
- Plan files là nguồn sự thật; marker `<!-- Red Team S1 F# -->` và `<!-- Updated: Validation Session 1 - ... -->` đánh dấu mọi chỗ sửa. Không hydrate task runtime.

## Next steps
- `/clear` rồi `/ak:cook /home/cesc/Documents/personal-workspace/teka/plans/260923-0715-giang-day-menu/plan.md` bắt đầu Phase 1.
- Thư mục rỗng `plans/_import/` chưa track, có thể xoá.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
