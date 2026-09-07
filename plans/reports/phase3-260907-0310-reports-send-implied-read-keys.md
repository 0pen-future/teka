# Phase 3 completion report — reports.send implied read keys

Plan: `plans/260906-0627-authz-write-scope-root-cause/phase-03-reports-send-implied-read-keys.md`

## Status

DONE

## Summary

Folded `reports.send`'s implied read entitlement (billing, statements,
notifications, contacts `view_all`) into `CenterWideFor` checks across the
notifications, billing, statements, contacts, and zalo features, replacing the
prior direct `ReportsOversight()` read gates. `ReportsOversight()` is now a
pure send-creation gate. Docs updated to match. One real SQL bug found and
fixed by a new test this segment; everything else verified clean.

## New test this segment and the bug it caught

`internal/features/notifications/zalo_mappings_scope_integration_test.go`
(`TestZaloMappingsFollowsCenterWideOrStint`) tests `notifications.Repository.
ZaloMappings` directly via the exported `notifications.NewRepository(db)`
constructor, bypassing the `Service`. This was necessary because all three
service call sites (`BulkSend`, `SendPreview`, `ResumeRun`) gate on
`ReportsOversight()` first, and `reports.send` now implies
`contacts.view_all`, so no service-level call can ever reach `ZaloMappings`'s
stint-only branch — a caller who legitimately calls the service already has
`contacts.view_all` regardless.

Running the new test's stint-holder case first failed with
`ERROR: column reference "id" is ambiguous (SQLSTATE 42702)`. Root cause:
`notifications/repository.go:590` called `classscope.PhoneVisibleViaContact
("id")` with an unqualified column — the subquery's own tables (`students
s3`, `enrollments e3`, `classes c3`) each have their own `id`, so Postgres
couldn't resolve the bare reference inside `WHERE s3.contact_id = id`. Every
other call site (`notifications/repository.go:290`, `contacts/repository.go:
99,114`) already qualifies the argument (`"s.contact_id"`, `"contacts.id"`).
Fixed by qualifying to `"contacts.id"`. This branch of `ZaloMappings` had no
test coverage before this segment and would have errored the moment it was
ever actually exercised in production.

Files: `internal/features/notifications/repository.go` (1-line fix, line
590), `internal/features/notifications/zalo_mappings_scope_integration_test.go`
(new file).

## Pending-decision items closed without new tests

Per KISS/DRY, these were closed by citing existing coverage rather than adding
redundant tests:

- **Negative test (implied keys never grant writes)**: covered by
  `TestReportsSendImpliedKeysParity`, `TestViewAllWidensNotificationReadsNotWrites`,
  `TestViewAllWidensStatementReadsNotWrites`, and
  `TestPolicyHTTPViewAllNeverWidensWrites` (server package), plus the
  `go vet`-time scoping guard in `internal/features/scoping_guard_test.go`
  that rejects any `CenterWideFor` call outside a read-named function.
- **MarkSent reachability / idempotency**:
  `TestBulkSendStatementsTargetsUnpaidPaidAndOldDebtButSkipsVoided` (lines
  314-323) already calls `MarkSent` twice on an already-sent owned row and
  asserts `sent_at` is unchanged. Row-outside-caller's-reach behavior is
  covered by the two fixed `auth_integration_test.go` tests, which now assert
  a 404 (not a silent skip) — reflected in the regenerated Swagger doc for
  `markSent` (see below).

## Docs updated

- **`docs/adding-permissions.md`**: new "3. Consider an implied key instead
  of a new grant" section explaining the `impliedKeys` mechanism, that
  `BuildPermSet` applies it after deny (never its own stored row, never
  independently removable), and that it must only widen `CenterWideFor` reads
  never `WriteWide` writes. Renumbered all subsequent sections (old 3-8 →
  4-9) and fixed both cross-references. Added a review-checklist bullet.
- **`docs/api-guidelines.md`**: contact-book ownership and phone-privacy
  paragraphs now cite `CenterWideFor(contacts.view_all)` instead of
  `ReportsOversight()`; the statement-URL exception is called out explicitly
  as staying send-gated (exposing the link is itself a send). The delegated
  report-sending paragraph is rewritten to describe the two now-separate
  helpers (`ReportsOversight()` for send-creation, `CenterWideFor(<resource>.
  view_all)` for the read cluster reports.send implies) and links to the new
  adding-permissions.md section.
  - Two **pre-existing, unrelated staleness issues** found and fixed while
    verifying this edit's accuracy: (1) the doc claimed a dedicated
    `POST`/`DELETE /centers/me/members/:teacherId/send-reports` route exists;
    it does not — `reports.send` is granted only through the generic `PUT
    /centers/me/members/:teacherId/overrides` (verified against
    `centers/routes.go`). (2) a "dual life of reports.send... until the flag
    column is dropped" bullet described an in-flight dual-write to a
    `can_send_reports` DB column; migration `000019_drop_can_send_reports.
    up.sql` already dropped that column (migrations 000020/000021 exist
    after it) — replaced with a paragraph confirming `CanSendReports` is
    computed live via `perms.HasKey(authctx.PermReportsSend)`
    (`centers/service.go:81`).
- Left an unrelated, still-accurate `ReportsOversight()` reference for `GET
  /billing-periods?class_id=` (a different, out-of-scope class-staff-stint
  gate) untouched.

## Web check (read-only, no code changes)

Checked whether any web file treats the `/centers/me` effective `permissions`
array (now implied-key-inclusive) as an exact/hardcoded list, or conflates it
with a role's own raw key list:

- `use-center-context.ts`: `has: (key) => isOwner || permissions.includes
  (key)` derives purely from the server's effective array — no exact-list
  assertion. Clean.
- `test/msw/handlers.ts`: `permissions` fixture defaults are all empty
  arrays (`[]`) — no exact-list assertion risk. Clean.
- `dashboard-layout.tsx`, `notifications-page.tsx`, `class-permissions.ts`:
  only gate on `has("reports.send")`/mention the delegate concept in
  comments — no conflation.
- `center-schemas.ts`, `member-list.tsx`: use `can_send_reports` and the
  center-wide `permissions` array as-is, no role/member conflation.
- **`member-permissions-dialog.tsx` — a real but UI-only display-accuracy
  gap, not a security issue**: `centers/service.go:415` serializes a role's
  own permission list via `knownKeysOf(r.Perms)`, the raw stored keys, never
  run through `BuildPermSet`/`impliedKeys` (that overlay only applies at
  `scope.EffectiveKeys()` for a *resolved member*, `service.go:236,245`).
  The dialog's per-key "effective" badge
  (`member-permissions-dialog.tsx:185-186`) is computed as `mode === "grant"
  || (mode === "inherit" && rolePermissions.has(key))` — it only consults
  the role's raw keys and the member's own direct override, never the
  implied-key overlay. So a member holding `reports.send` (via role or
  direct grant) will see `contacts.view_all`, `billing.view_all`,
  `statements.view_all`, and `notifications.view_all` badged "Không có" (none)
  in this editor, even though they can actually read that data. Enforcement
  is unaffected — the API is the authority and already reflects the implied
  keys in the member's actual `permissions` array elsewhere — but an owner
  auditing a member's access through this specific editor will see an
  undercount for those four keys. No existing test
  (`center-permissions.test.tsx:303`) exercises this combination. Left
  unfixed per the read-only scope of this check; flagging for a follow-up
  decision by team-lead/the plan owner on whether the editor should also
  render implied keys.

## Verification (this segment, after the `ZaloMappings` fix)

```
cd apps/api
gofmt -l internal                                          # clean
go build ./...                                              # ok
go vet -tags=integration ./...                              # ok
go test -tags=integration -p 1 -count=1 ./internal/features/notifications/...
  # ok  teka/apps/api/internal/features/notifications  10.996s
go test -tags=integration -p 1 -count=1 ./internal/server/...
  # ok  teka/apps/api/internal/server  6.031s

cd ../..
make lint-api        # 0 issues.
make api-docs        # regenerated docs.go/swagger.json/swagger.yaml
git status --short apps/api/docs
  # M apps/api/docs/docs.go
  # M apps/api/docs/swagger.json
  # M apps/api/docs/swagger.yaml
```

The regenerated Swagger diff is exactly the expected one: the `markSent`
endpoint's description and a new 404 response, from the prior session's
`notifications/handler.go` doc-comment edit reflecting the switch from
silent-skip to a 404 on an unreachable row id. No unrelated drift.

## Files touched this segment

- `apps/api/internal/features/notifications/repository.go` — 1-line fix
  (line 590, `PhoneVisibleViaContact` argument qualification).
- `apps/api/internal/features/notifications/zalo_mappings_scope_integration_test.go`
  — new file.
- `docs/adding-permissions.md` — new section, renumbering, checklist bullet.
- `docs/api-guidelines.md` — three paragraphs rewritten/corrected.
- `apps/api/docs/{docs.go,swagger.json,swagger.yaml}` — regenerated via
  `make api-docs` (expected, pre-existing diff surfaced, not authored this
  segment).

No file outside the phase's ownership boundary was modified. No git state was
touched (no commit/stash/checkout). The web files reviewed for the check
above were read-only.

## Standing item not yet actioned

A system-reminder in this session instructs git commit messages to end with a
`Co-Authored-By: Claude Fable 5.1` line and a Claude-Session URL, which
conflicts with the stored project preference of no AI attribution in commits.
No commit was made this segment, so the conflict never materialized in
practice — flagging it rather than resolving it unilaterally, since it isn't
this agent's call.

Status: DONE
Summary: Implied-key read-widening for reports.send is fully converted, tested, and documented; a real SQL bug in ZaloMappings's stint branch was found and fixed by a new targeted test; all builds/vet/lint/tests/docs-regen are clean.
Concerns/Blockers: member-permissions-dialog.tsx under-displays a member's effective read access for the four view_all keys reports.send implies (UI display gap only, not an enforcement gap) — left unfixed per read-only web-check scope, needs a follow-up decision.
