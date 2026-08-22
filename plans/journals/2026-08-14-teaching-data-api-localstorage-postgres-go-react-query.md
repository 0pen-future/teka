---
title: "Teaching data API: localStorage → Postgres + Go + React Query"
date: 2026-08-14
summary: "Migration 000009, Go teaching feature (12 endpoints), React Query hooks replacing the web local store; 6 review findings fixed; shipped in 4 commits"
---

# Teaching data API: localStorage → Postgres + Go + React Query

## What happened

Executed plan `plans/260814-1345-teaching-data-api/` end to end in auto mode. Teaching data (curricula, lesson plans with the owner review loop, session marks, session notes) moved from the web app's localStorage store to:

- Postgres migration 000009 (`curricula`, `lesson_plans`, `session_marks`, `session_notes`).
- Go `teaching` feature package — 12 endpoints, server-authoritative lesson-plan state machine (illegal transition → 409), tri-state marks merge (omitted/null/value; both-NULL row deleted).
- React Query hooks replacing the local store with optimistic mutations; UI components unchanged.

Finalize chain ran with parallel subagents: code review (DONE_WITH_CONCERNS, no blockers), tester (all green: web 336/336, tsc, eslint; go vet/test; 13/13 integration), plan sync-back.

## Review fixes

- H1: DTO size bounds via gin binding tags; `Optional[T]` bounds enforced in `validateMarkEntries` (rune-counted, batch ≤100, note ≤1000).
- H2: success toast + draft reset moved into mutate-level `onSuccess` — a failed save keeps the user's draft editable (regression test added).
- M1: concurrent first-save race on `uq_lesson_plans_class_lesson` now 409 via `gorm.ErrDuplicatedKey` instead of 500.
- M2: dead `useReviewQueue` export deleted.
- M3: `cancelQueries` before optimistic cache writes.
- M4 (user decision "always-correctable"): `PutMarks` roster gate now only guards NEW-row creation — an existing row stays editable/clearable after the enrollment ends (`TestMarksExistingRowSurvivesUnenrollment`; swagger regenerated).

## Decision

Marks history is teacher-correctable, not frozen with the roster: a since-unenrolled student's existing mark row can be edited or cleared; creating a new row still requires roster membership on the session date.

## Commits

- 271e351 feat(db): add teaching database migration
- b4f1516 feat(api): implement teaching feature with service, handlers, and repository
- f0e5f13 feat(web): replace teaching data store with React Query API integration
- c594fd0 docs: add teaching-data-api plan and project reports

## Next steps

- Live-stack demo of the review loop (integration tests cover it; a manual two-account run remains outstanding).
- Deferred Low findings recorded in `plans/reports/code-review-260814-1355-teaching-data-api.md`.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
