---
title: "Plan: Sửa lớp học modal replaces class settings screen"
date: 2026-09-24
summary: "Planned the v5 edit-class dialog and removal of /classes/:id/settings; design file truncated at 256KB"
---

# Plan: Sửa lớp học modal replaces class settings screen

## What happened
- Imported So Lop v5 via claude_design (DesignSync get_file). The file is ~260KB and get_file caps at 256KB (`truncated: true`); a re-fetch returned identical content. The "Sửa lớp học" modal markup and the prototype logic sit past the cut, so they could not be read.
- Scouted the roster feature: the "Sửa" button in `class-table.tsx` links to `/classes/:id/settings` (`ClassSettingsPage`), which duplicates class editing. Other links to it: the detail header, the info-tab shortcut, and `classes-tab.tsx`.

## Decision
- Plan `plans/260924-1703-sua-lop-hoc-modal/` (3 phases): `ClassDialog` gets `mode: "edit"` (following the course-dialog pattern), and the save fan-out moves into `useSaveClassSettings`. Every entry point opens the dialog, and `/classes/:id/settings` redirects to `/classes/:id?edit=1`.
- The owner-only `ClassStaffSection` and `TeacherHandoffCard` (`#teacher-handoff`) move to the class detail info tab (user choice).
- Edit fields default to name, weekly slots and unit price, pending a design reconciliation gate in Phase 1.

## Next steps
- Get the tail of the v5 file (user paste or export) to confirm the modal's fields, then `/ak:cook` the plan.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
