---
title: "Danh sách lớp học v5 UI"
date: 2026-09-24
summary: "Restyled /classes and the edit modal to the So Lop v5 prototype; modal fields unchanged because the design file is truncated"
---

# Danh sách lớp học v5 UI

## What happened
- DesignSync `get_file` still truncates v5 at 256KB. The list screen markup was fully
  readable. For `modalClass`, only the start is readable in v5, but v4 has it in full.
  The dedicated edit-modal markup and the prototype logic (`cl.*` bindings) were
  not readable.
- Rebuilt `ClassTable` as the prototype's 8-column table. It is still a real `<table>`,
  so row, keyboard, and test semantics survive. Added `formatScheduleLines`, which
  mirrors the API `ShiftOf` bands for the dot colours.
- Added a "Mở" button so the row can be opened without a mouse. The class name stays a
  link because the invitations e2e depends on it.

## Open
- If the v5 modal Sửa adds fields (course, dates), they already exist in
  `classUpdateInputSchema`, so adding them needs no API change.

> Historical work record — not durable authority.
