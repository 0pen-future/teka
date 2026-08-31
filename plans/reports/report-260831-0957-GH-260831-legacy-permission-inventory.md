# Legacy permission inventory — Phase 8 entry snapshot

Date: 2026-08-31 (read-only SELECT against production `teka-db`, approved by
operator; counts and permission keys only, no personal data).

Binary under soak: commit `fad8b88` (catalog v2, enforcement live since
2026-08-31 ~02:20). Soak evidence at decision time: ~7.5h, 39 authenticated
requests, structured denial log empty, no 5xx.

## Legacy-row inventory (by table, whole database)

| Check | Query shape | Result |
|---|---|---|
| Legacy `data.*` keys in role assignments | `center_role_permissions WHERE permission_key LIKE 'data.%'` | **0 rows** |
| Legacy `data.*` keys in member overrides | `center_member_permissions WHERE permission_key LIKE 'data.%'` | **0 rows** |
| Per-resource `*.view_all` rows (incl. reserved `scores.view_all`, `teaching.view_all`) | `permission_key LIKE '%.view_all'` in both tables | **0 rows** |
| Keys outside catalog v2 | anti-join vs the 62-key registry | **0 rows** |
| Member override denies | `center_member_permissions WHERE effect = 'deny'` | **0 rows** |
| Member override grants | `center_member_permissions` | **1 row**: `reports.send` grant (dual-written by the legacy send-reports flow; matches `can_send_reports = true` on the same active stint) |

Population: 5 centers × 3 system roles each (15 roles, all carrying the
53-key canonical baseline), 8 active memberships.

## Parity gates (all must be 0 — all are 0)

| Gate | Meaning | Result |
|---|---|---|
| `send_reports_drift` | `center_members.can_send_reports` ↔ effective `reports.send` disagree on an active stint | 0 |
| `class_gv_missing_stint` | `classes.teacher_id` points at a teacher without an active `giao_vien` stint on that class | 0 |
| `class_zero_active_gv` | class with zero active `giao_vien` stints but a non-null `teacher_id` | 0 |

## Exception resolution (task: "resolve every exception")

No exceptions exist. There are no legacy grants, no legacy denies, no unknown
keys, and no alias-dependent rows in production. The single member override is
already canonical (`reports.send`) and parity-consistent.

## Backup / provenance for the destructive steps

- The "exact legacy-row backup" required by the fail-safe sequence is **this
  inventory**: the set of rows matching the three retired keys
  (`data.view_center_wide`, `scores.view_all`, `teaching.view_all`) is empty,
  so the backup is the empty set, with provenance recorded here.
- Migration 000019 (drop `center_members.can_send_reports`) is reversible: its
  down migration restores the column and backfills it from `reports.send`
  override rows — which is exactly how the one live grant is represented.
- Migration 000020 (delete retired-key rows) deletes 0 rows in production by
  this inventory; its down is a documented no-op backed by this snapshot.
- Independent rollback points = previous image tag (`fad8b88` stamp) plus the
  per-migration down files.

## Decisions recorded

- `scores.view_all` / `teaching.view_all`: **retire, not wire.** No
  enforcement site exists (grading/teaching scope via class/session
  resolution), zero rows hold them, and pre-catalog behavior had no
  per-resource denial there — wiring them now would invent behavior nobody
  asked for (YAGNI). Re-adding later is an additive catalog change.
- Alias removal and row cleanup ship as separate commits/migrations to keep
  the "separately deployable" property despite the compressed sequence the
  operator approved.
