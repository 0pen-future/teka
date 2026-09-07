# Prod inventory — `view_all` holders and non-owner writes (2026-09-06)

Read-only counts from the production database (`teka-db`, `psql -U teka -d teka`),
run before deploying the write-scope fix from
[`260906-0627-authz-write-scope-root-cause`](../260906-0627-authz-write-scope-root-cause/plan.md).
No personal data was printed; every query aggregates.

## Who holds a `<resource>.view_all` key

| Source | Rows | Centers |
|---|---|---|
| `center_role_permissions` with `permission_key LIKE '%.view_all'` | 0 | 0 |
| `center_member_permissions` (allow or deny) with `permission_key LIKE '%.view_all'` | 0 | 0 |
| live `center_members` whose role grants any `view_all` | 0 | 0 |

No member in production holds a visibility key today, by role or by override.
The escalation path (a `view_all` key widening a write) has therefore never
been reachable in production; the fix closes a latent defect, not an exploited
one.

## Non-owner writes on money and class resources, last 60 days

`audit_logs` where `actor_role <> 'owner'`, mutating method, `status_code < 300`,
action prefix in billing / payments / statements / sessions / classes /
enrollments / notifications:

| Action | Writes | Distinct actors | Centers |
|---|---|---|---|
| `billing.period.create` | 27 | 2 | 2 |
| `billing.period.draft` | 6 | 2 | 2 |

Both actions create or draft the actor's **own** period (`EnsurePeriod` /
`Draft` anchor on `sc.TeacherID`), which the fix leaves untouched. With zero
`view_all` holders there is no row in this window that a member could have
reached through a visibility key.

## Decision confirmed

- No data migration or backfill is needed (plan decision D4): nothing was
  written across tenants, and no key assignment has to be revoked.
- The deploy is a plain code rollout; no operator action follows.
