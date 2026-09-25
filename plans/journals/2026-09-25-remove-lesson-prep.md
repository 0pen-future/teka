# Chuẩn bị tài liệu: lesson prep removed end to end

**Date**: 2026-09-25
**Plan**: plans/260925-0939-remove-lesson-prep/plan.md

## What shipped

- DB: migration 000036 deletes the `prep.assign` rows from `center_role_permissions` and `center_member_permissions`. It then drops `idx_template_lessons_assignee`, `fk_template_lessons_assignee` and the `prep_status`, `assignee_id`, `due_date` and `checklist` columns from `template_lessons`.
- API: removed `GET /library/versions/:vid/board`, `PATCH /library/lessons/:lid/prep`, `PATCH /library/lessons/:lid/assignment` and `GET /library/assignees`, along with the prep fields on lessons and `TemplateResponse.prep`. `PermPrepAssign` left the catalog, and `CatalogVersion` went to 6.
- Web: removed the `/prep*` pages, the lesson prep panel, `use-prep`, the prep API, schemas, labels and MSW handlers, the nav entry and `e2e/prep.spec.ts`. The template wizard now always lands on the template detail page.

## Decisions

- Down migration restores the schema only, so older binaries can still run. The prep data and the grants removed on up are gone for good.
- The CatalogVersion bump is required because the meaning of stored assignments changed. Stale clients then fail the CAS check instead of re-sending `prep.assign`.
- Old audit rows with `template_lesson.prep` and `template_lesson.assign` stay in the database as history.
- The shared Kanban component and the tasks board are untouched.

## Verification

- API: `make lint-api` (0 issues), `make test-api-unit`, serial integration tests (51 packages, coverage 78.6%), `make api-docs`.
- Web: typecheck, eslint, prettier, vitest 1159 passed. e2e was not run because the local teka-* stack is production.
