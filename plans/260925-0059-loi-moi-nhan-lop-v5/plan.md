---
title: "Lời mời nhận lớp theo So Lop v5 (API + web)"
status: completed
created: 2026-09-25
branch: feat/giang-day-menu
---

# Lời mời nhận lớp

## Outcome
The "Lời mời nhận lớp" screen matches the v5 prototype `screen 'invites'`:
- Subtitle: "Giáo viên được mời vào lớp phải xác nhận trước khi xuất hiện trong Đội ngũ giảng dạy."
- Chips: Tất cả, Chờ xác nhận, Đã nhận, Từ chối. Cancelled rows show only under Tất cả.
- Columns: STT, Lớp học (with the course name, or "Chưa gắn khóa"), Giáo viên,
  Gửi lúc, Trạng thái, Thao tác.
- Owner actions on open rows: GV nhận lớp (Phân công for other roles), Nhắc lại
  (pending only), Hủy.
- Empty label: "Không có lời mời nào." shown inside the table. The card has a
  20px radius and shadow-md.

## Design source
- `So Lop - Prototype v5.dc.html` and its logic (the invites screen), plus the
  `_ds` tokens, pulled with DesignSync.

## API contract
- `InvitationResponse.course_name` (nullable string) is the class's course, taken
  from a `LEFT JOIN courses` that skips soft-deleted courses.
- No migration: `classes.course_id` already exists.

## Web
- `classInvitationSchema` gains `course_name`.
- `class-invitation-labels.ts` holds the prototype labels and the four-chip view model.
  "Đã nhận" covers both accepted and assigned rows.
- `class-invitations-page.tsx` uses the prototype table layout. These repo semantics stay:
  - the role label under the teacher's name;
  - the member's Chấp nhận and Từ chối actions;
  - the "Đã đồng ý" badge for accepted rows;
  - the invite message shown under the class;
  - the loading and error blocks.

## Acceptance
- [x] The integration test `TestInvitationCarriesTheClassCourse` covers three cases:
  no course, a linked course, and a soft-deleted course.
- [x] Web tests cover the layout, headers, course line, chips, owner action order and labels.
  Typecheck passes, the full vitest run passes (123 files, 1176 tests), and lint adds no new warnings.
- [x] `class-invitations.spec` passes on the isolated `teka-e2e` stack, which was then torn down.
- [x] Deployed on 2026-09-25 at 01:10 with the production compose (`-p teka`, prod + homelab, `.env.production`):
  - Images are `teka-{api,web}:5d4f3a2-dirty-260925-0110`; the rollback images are `pre-260925-0110`.
  - `migrate` was a no-op, so no dump was needed; `/readyz` returns 200.
  - The bundle contains the new copy.
  - The first attempt failed on a zsh `$s:l` tag expansion. It was fixed with `${s}` and redeployed.
