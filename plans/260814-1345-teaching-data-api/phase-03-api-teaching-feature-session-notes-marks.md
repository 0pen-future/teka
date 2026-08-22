---
phase: 3
title: "API teaching feature: session notes & marks"
status: completed
priority: P1
effort: "1d"
dependencies: [1, 2]
---

# Phase 3: API teaching feature: session notes & marks

## Overview

Extend the `teaching` package with the month-batched marks read and the session-scoped note/marks writes (endpoints 10–12 of the contract table).

## Requirements

- Functional: `GET /classes/:id/marks?month=YYYY-MM`, `PUT /sessions/:id/note`, `PUT /sessions/:id/marks` (batch upsert).
- Non-functional: one query per read path (no N+1 across sessions); writes idempotent (safe under debounce retries from the web); score range validated 0–10.

## Architecture

- **Read shape** mirrors what the web adapter needs to rebuild `TeachingState` slices:
  ```json
  {
    "session_notes": [{"session_id": "...", "body": "..."}],
    "marks": [{"session_id": "...", "student_id": "...", "score": 8.5, "personal_note": "..."}]
  }
  ```
  Month filter joins `class_sessions` on `class_id` + `session_date` within the month (reuses `idx_class_sessions_class_date`), then notes/marks by session id. Access: class teacher or owner (read-only for owner), same convention as Phase 2.
- **Write semantics**: `PUT note` upserts by PK, empty `body` deletes the row. `PUT marks` takes `[{student_id, score?, personal_note?}]`: per row, provided-null clears that field, omitted field is left untouched, and a row whose resulting `(score, personal_note)` are both NULL is deleted — keeps the table free of empty rows without a separate DELETE endpoint. Validate every `student_id` is on the session's roster (enrollment window), reject foreign students 400/404 per repo convention.
- Writes are session-teacher-only (`class_sessions.teacher_id == scope.TeacherID`); owner does not write marks (matches UI).
- Batch upsert in one statement (`INSERT ... ON CONFLICT (session_id, student_id) DO UPDATE`) inside a transaction with the roster check.

## Related Code Files

- Modify: `apps/api/internal/features/teaching/{dto,handler,repository,routes,service}.go` (+ tests)
- Modify: `apps/api/docs/` (swagger regen)
- Read (evidence): `apps/api/internal/features/attendance/` (roster resolution for a session), `sessions` feature (month filtering convention if one exists)

## Implementation Steps

1. Read how `attendance` resolves a session's roster (present/absent per enrollment) — reuse its repository approach for the roster check.
2. Implement marks/notes read with the two-part response; integration test with a seeded month (sessions in/out of range, note-only session, marks-only session).
3. Implement note upsert/delete-on-empty; marks batch upsert with clear/delete semantics; table-driven service tests for the field-merge rules.
4. Swagger + `go test ./...`.

## Success Criteria

- [x] Month read returns exactly the sessions in the requested month; cancelled sessions included only if the UI includes them today (check `use-month-sessions.ts` and match — do not invent a filter).
- [x] Upsert merge rules covered: set score only, set note only, clear one, clear both → row gone.
- [x] Roster check rejects a student not enrolled for that session.
- [x] Owner can read, cannot write (403); other-teacher hidden per convention.

## Risk Assessment

- **Debounce race** — two rapid PUTs for the same student: last-write-wins per field is acceptable (single editor per class in practice); no version column (YAGNI), noted for revisit if co-teaching lands.
- **Roster-check cost** — one extra query per batch write, bounded by class size; acceptable.

## Completion notes

- Implemented as designed: month-batched read, note upsert/delete-on-empty, marks batch upsert with clear/delete-on-both-null semantics, roster check rejecting foreign students.
- `go vet ./...`, `go test ./...`, and `go test -tags integration ./...` all green for this feature.
