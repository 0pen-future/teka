---
title: Plan 07 web teacher app completed
date: 2026-08-04
summary: "Teacher web app (auth shell, roster, attendance, billing close, collections, notifications) delivered; recovered orphaned roster/attendance commit"
---

# Plan 07 web teacher app completed

## What happened

Completed all four phases of the teacher-facing web app on top of the V1 API:
- Phase 1: phone-login rework, app shell, pending-attendance dashboard warnings.
- Phase 2: roster (classes, students, contacts, enrollments) with schedule editor and money input.
- Phase 3: session list and one-touch attendance with dirty-guard and closed-period warning.
- Phase 4: billing close (chốt sổ) review screen, collections board (by-contact / by-class), record-payment with allocation editor, and fee-notification composer with copy-to-clipboard.

Phase 4 API divergences from the plan were consolidated into adr.md: review reads through POST /draft (open) or GET /preview (closed) with a 409 fallback; blocking-session gate via GET /sessions/pending; no allocation-preview endpoint (record-then-correct); global mark-sent; by-class requires class_id.

## Decision

- Review query is modeled as a read-driven query gated on the period status: open → draft (persists invoices, returns real invoice_id), closed → preview (pure read, invoice_id always null). refetchOnWindowFocus disabled so a tab refocus never re-issues draft's write. A 409 fallback self-heals the window right after a close.
- Collections fee-notification generation is an explicit user action (button), never an on-mount effect, to avoid firing a bulk write from render.

## Recovery note

During finalize, `git status` revealed the roster/attendance feature source and their e2e specs as untracked: the earlier phase 2-3 commit had become orphaned (main was rebuilt onto the phase-1 base, and the billing commit landed directly on it). Verified the on-disk files were byte-identical to the orphaned commit (staged diff empty) and that the full suite passed against them, then re-committed them onto main so nothing was lost.

## Validation

- Typecheck clean (tsc -b --noEmit exit 0).
- Full web suite green: 21 files, 79 tests.
- Design-system hex gate clean across billing and collections.

## Next steps

- Plan 08: web parent statement page (depends on the design system; uses StatusPill and ProgressBar).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
