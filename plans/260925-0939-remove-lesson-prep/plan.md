---
title: "Remove Chuẩn bị tài liệu (lesson prep) from API, DB and web"
status: completed
created: 2026-09-25
branch: feat/remove-lesson-prep
---

# Remove Chuẩn bị tài liệu

## Outcome
The "Chuẩn bị tài liệu" feature is gone end to end: the `/prep` pages, the prep
board, the assignment screen, the lesson prep panel, the `prep` summary on
templates, the `prep.assign` permission, the prep endpoints and the four prep
columns on `template_lessons`.

## Scope
- DB: migration `000036_drop_template_lesson_prep`.
  - up: delete `prep.assign` rows from `center_role_permissions` and
    `center_member_permissions`; drop `idx_template_lessons_assignee`,
    `fk_template_lessons_assignee`, and the `prep_status`, `assignee_id`,
    `due_date`, `checklist` columns.
  - down: re-add the columns, FK and index exactly as 000032 created them.
    The dropped values and permission rows are not restored.
- API (`library`): remove `GET /library/versions/:vid/board`,
  `PATCH /library/lessons/:lid/prep`, `PATCH /library/lessons/:lid/assignment`,
  `GET /library/assignees`; the prep fields on `LessonResponse`; `TemplateResponse.prep`;
  the draft prep summary query. Remove `PermPrepAssign` from the catalog and bump
  `CatalogVersion` to 6. Update routespec, the route policy snapshot, audit
  action tests, seeds, migrations_test step counts and the swagger docs.
- Web: remove the `/prep*` routes and pages, `use-prep`, `lesson-prep-panel`,
  the prep API/schemas/labels/MSW handlers, the `prep` permission group label,
  the nav entry, and `e2e/prep.spec.ts`. The template wizard always lands on the
  template detail page.
- Docs: drop the prep board mentions in `docs/api-guidelines.md` and
  `docs/frontend-guidelines.md`.

## Non-goals
- No change to the shared Kanban component or the tasks board.
- No production migration run in this task.

## Acceptance
- [x] No reference to prep / prep.assign / prep_status / assignee_id on
      template lessons remains in apps/ (except migrations 000032 and 000036).
- [x] `make test-api` (serial), Go lint/vet, and swagger regeneration pass.
- [x] Web typecheck, lint, vitest pass (e2e not run: local stack is production).
- [x] Migration up/down round-trips in the migrations test.

## Result
- API: `make lint-api` 0 issues; `make test-api-unit` and serial integration
  tests (51 packages) pass; swagger regenerated.
- Web: typecheck, eslint, prettier, vitest (1159 passed) pass. The library e2e
  specs were not run because the local teka-* stack is production.
- Migration 000036 permanently drops prep data and `prep.assign` grants; down
  restores the schema only.
