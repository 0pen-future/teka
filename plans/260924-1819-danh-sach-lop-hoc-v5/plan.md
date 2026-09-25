---
title: "Danh sách lớp học + Sửa lớp học theo So Lop v5"
status: completed
created: 2026-09-24
---

# Danh sách lớp học + Sửa lớp học theo So Lop v5

## Outcome
The `/classes` screen and the "Sửa lớp học" modal match the v5 prototype
(`So Lop - Prototype v5.dc.html`, screen `data-screen-label="Danh sách lớp học"`, and `modalClass`).

## Constraints / non-goals
- The API contract does not change: filters, stats, and save fan-out stay the same.
- The edit fields stay as D4 in `plans/260924-1703-sua-lop-hoc-modal/`. The v5 file is
  truncated at 256KB (the modal Sửa markup is not readable), so no new fields are added.

## Changes
- Header: a subtitle, a 44px "Tải lại" button (refetches list + stats), and "+ Lớp học".
- A white card (radius 20, shadow-md) holding the day/shift selects, the search box
  "Tìm kiếm theo tên, mã lớp học", and the status chips.
- The table uses the prototype's 8 columns:
  - STT
  - Lớp học (plus a course · code line)
  - Lịch học: one line per session, "Thứ Ba, 19:00 - 20:30", with a dot coloured by shift
    (API bands: <12:00, <17:30)
  - Gắn thẻ: sky pills, or "-"
  - Trạng thái
  - the two dates
  - Sửa / Mở
- The empty note sits inside the table.
- Modal: the khung-giờ list is capped at 280px with scrolling, and the slot card
  uses a 1.5px border with radius 16. The hint text that is not in the design was removed.

## Acceptance
- [x] roster + teaching vitest, typecheck, and eslint (0 errors)
- [x] Playwright: class-list, class-staff-read/write, class-invitations (isolated stack)
- [x] Screenshots checked by eye against the v5 markup
