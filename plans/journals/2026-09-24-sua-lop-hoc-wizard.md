# Sửa lớp học: six-section wizard with its backend

**Date**: 2026-09-24
**Plan**: plans/260924-2240-sua-lop-hoc-wizard/plan.md

## What shipped

- Web: `ClassEditWizard` replaces the edit dialog. It has six sections with a section nav: info, links, study mode, weekly schedule, planned resources and notes. It saves in this order: PUT, then schedule adds, closes and deletes, then the teacher invitation. A failure part-way through gives a partial result, which refetches the class so a retry diffs against what already landed.
- API: migration 000035 adds `room` and `study_mode`. Classes gain `next_class_id`, which sets the child's `parent_class_id`. New `GET /classes/availability` lists free rooms and teachers for a set of slots. A self-paced class rejects schedules with 422 SELF_PACED_NO_SCHEDULE. The API also validates end_date ≥ start_date and duration_min between 1 and 600.

## Decisions

- The prototype's wizard markup was truncated in the design export, so the UI was rebuilt from its logic spec: sections, quick presets and the free-room and free-teacher searches.
- Course stays optional. Legacy classes created without a course must still save; the blank choice only shows for classes that never had a course.
- Re-picking the current course keeps a class-specific price. A changed price shows the "applies from now on" warning.
- Teachers with a pending invitation are hidden from the planned-teacher picker, because the API refuses a second invitation.

## Verification

- Web: typecheck, lint (0 errors), prettier, vitest 1166 passed.
- API: build and vet, `make test-api-unit`, serial `make test-api` (coverage 78.6%), `make api-docs`.
- Two mechanical test fixes outside the feature: the route-policy snapshot gained the availability route, and the migrations rollback step count went from 30 to 31.
