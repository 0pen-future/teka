---
phase: 5
title: "Hồ sơ học sinh & chi tiết"
status: completed
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 5: Hồ sơ học sinh & chi tiết

## Overview

Build `/records` (student records list, prototype lines 486–523) and `/records/:studentId` (single-student record, lines 526–578): score averages, trends, absence counts, per-session score bar chart, inline personal notes, CSV exports. NGÀY SINH renders "—" everywhere (decision #2); birthday banner/🎂 affordances are omitted.

## Requirements

- Functional — list page: title + subtitle; `Tải danh sách (CSV)` outline button; class search + pill tabs (same component as classbook); table card (grid `1fr 110px 84px 110px 70px 100px`) with columns HỌC SINH / NGÀY SINH ("—") / ĐIỂM TB (colored by band per prototype styles) / XU HƯỚNG (arrow glyph + label from trend calc) / VẮNG (absence count) / `Xem hồ sơ` outline button → detail route. No birthday banner (requires dob data that does not exist).
- Functional — trend rule (prototype `trendOf`, lines ~1700+): needs ≥4 scores; compare mean of first half vs last half (k = floor(n/2)); delta > +0.4 → "Tiến bộ" (mint ↑), < −0.4 → "Cần kèm" (coral ↓), else "Ổn định" (ink →). Fewer than 4 scores → "Chưa đủ dữ liệu".
- Functional — detail page: `← Hồ sơ học sinh` back link (sky); 54px sky avatar disc with initial; name + sub line (class · nhập học date); `Tải hồ sơ (CSV)` button; 3 stat cards (ĐIỂM TB, XU HƯỚNG, VẮNG with subs); score bar chart card ("ĐIỂM KIỂM TRA TỪNG BUỔI — THÁNG <MM>", horizontal-scroll bar row: value label / height-scaled bar / session label); sessions table (grid `84px 64px 64px 1fr`: BUỔI / BÀI / ĐIỂM mark chip / NHẬN XÉT CÁ NHÂN inline borderless input, saved to store on blur).
- Functional — CSV exports (shared `lib/csv.ts`): list = one row per student with avg/trend/absences; detail = one row per session with score + note.
- Non-functional: absences from real attendance data (confirmed sessions where student absent); scores/notes from the store; both pages handle empty-score states gracefully ("Chưa đủ dữ liệu", empty chart placeholder).

## Architecture

- Pure derivation module `lib/student-stats.ts`: `trendOf(scores)`, averages, absence aggregation — unit-tested against the prototype's thresholds including boundary cases (exactly ±0.4, n=3 vs n=4).
- List page needs per-student score/absence aggregates across the class's sessions: reuse the session + roster queries from Phase 3 (same query keys → React Query cache dedupes across the two pages; no new fetch layer). Student list itself comes from existing roster/enrollment hooks filtered by selected class.
- Detail page keys by `studentId` route param; class context resolved from the student's enrollment (fall back to a class picker only if a student has multiple active enrollments — match whatever the roster detail page already does).
- Bar chart is plain flex divs with token colors, as in the prototype — no chart library (KISS; the DS look *is* hand-sized divs).

## Related Code Files

- Modify: `apps/web/src/features/teaching/pages/records-page.tsx`, `student-record-page.tsx`
- Create: `apps/web/src/features/teaching/components/student-records-table.tsx`, `score-bar-chart.tsx`, `student-sessions-table.tsx`
- Create: `apps/web/src/features/teaching/lib/student-stats.ts` (+ unit tests)
- Reference: `apps/web/src/features/roster/pages/student-detail-page.tsx` (existing patterns), `lib/csv.ts` from Phase 3

## Implementation Steps

1. Implement `student-stats.ts` with unit tests first (trend thresholds, insufficient data, absence counting on unconfirmed sessions).
2. Build list page: tabs, aggregate rows, trend/avg styling bands, CSV export.
3. Build detail page: header block, stat cards, bar chart, sessions table with blur-saved inline notes.
4. Wire cross-links: `Xem hồ sơ` → detail; back link → list preserving selected class (search param).
5. msw/component tests: list renders aggregates from mocked data + store scores; inline note persists on blur; NGÀY SINH cell is "—"; exports produce expected CSV text.

## Success Criteria

- [x] Both screens match prototype layout/styles; trend arrows and score bands colored per DS tokens.
- [x] Trend/absence math unit-tested including boundaries; empty states render.
- [x] Inline notes persist per student+session across reloads.
- [x] NGÀY SINH shows "—"; no dob field is read, stored, or requested anywhere.
- [x] Suite green.

## Risk Assessment

- **Aggregate cost on large classes** (sessions × students) → derivations run on already-cached query data in a memoized selector; fine at this product's scale (tens of students, ~13 sessions/month). If it ever grows, the pure module is the isolation point to optimize.
- **Route-param student not in selected class** → resolve class from the student's own enrollment, not the list page's tab state.
