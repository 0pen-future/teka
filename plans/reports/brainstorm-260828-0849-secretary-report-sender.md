# Brainstorm: Secretary as delegated report sender (own Zalo)

Status: contract accepted 2026-08-28. Feeds next plan. Decisions confirmed by
user via 3-question gate.

## Contract

**Outcome:** Center owner invites a secretary as a center member with her own
Teka account. Secretary links her own personal Zalo and can send statement /
reminder reports to contacts (parents) for every teacher in the center, from
her own Zalo account. Owner no longer has to send personally.

**Constraints:**
- Keep 1-Zalo-per-person invariant (`zalo_accounts` PK = teacher_id) and
  per-person consent model. No center-level or second-account Zalo linking.
- Secretary permission = read statements/debt + send statement/reminder for
  all center members. NO write access to attendance, classes, billing config,
  contacts, membership.
- Sends reuse existing contact mapping (`contacts.zalo_user_id` chosen from
  the mapping teacher's friend list). Secretary must befriend parents first;
  reuse existing friend-request flow; pre-send warning for non-friend targets.
- Audit must attribute sends to the secretary (actual sender), not owner.
- Backward compatible: existing owner-oversight send path keeps working.

**Non-goals:**
- Center-level shared Zalo account (rejected; reverses migration 000007
  decision "Zalo đi theo người").
- Second Zalo under owner account (rejected; consent + PK violation, secretary
  session credentials would sit under owner login = takeover material).
- Secretary re-mapping contacts to her own friend list.
- General role/permission framework beyond what this needs (YAGNI).

**Acceptance criteria:**
- Owner can invite secretary (invitations flow) and grant/revoke the
  send-reports permission.
- Secretary logs in, links own Zalo (existing per-teacher link flow untouched).
- Secretary can list periods/statements across the center and run a paced send;
  messages go out from her Zalo; run/notification rows attribute her as sender.
- Secretary cannot mutate attendance/classes/billing/contacts (403 paths
  tested).
- Existing owner send-own-Zalo behavior unchanged (regression tests pass).

## Chosen direction

Option A of three: secretary = center member with own account + own Zalo +
delegated "send reports" permission. Smallest approach satisfying contract;
aligns with every existing invariant.

Evidence anchors:
- `apps/api/migrations/000004` — zalo_accounts PK teacher_id, sealed creds,
  consent per person.
- `apps/api/migrations/000007` — center tenancy; Zalo deliberately NOT
  re-keyed to center; invitations onboard members.
- `authctx.Scope{TeacherID, CenterID, IsOwner}` resolved per request —
  permission extension point (IsOwner → capability flags).
- `notifications/service.go:415-419` — sender always uses own Zalo slot even
  under oversight; template for secretary path.
- `zalo/service.go:514` — friend-request flow exists for friend-gap.

Rejected options (record): B = second Zalo under owner (security/consent
violation); C = center-level Zalo (scope blowout, reverses recorded design
decision).

## Key risks

- Zalo may throttle/block DMs from a fresh account to many non-friends
  (spam heuristics). Mitigation: befriend-first UX + pre-send warning; pacing
  already exists in notification runs.
- Authorization surface: adding capability besides IsOwner touches middleware +
  every send/read path guard — needs focused tests per path.
- Role naming: V1 issues only teacher accounts (`authctx` comment). Secretary
  likely stays a "teachers" account with a membership permission flag, not a
  new JWT role — decide in plan.

## Unresolved questions

- UI surface for owner to grant/revoke the permission (center settings page?)
  — decide during plan.
- Whether secretary sees statements read-only pages identical to owner
  oversight views or a trimmed list — decide during plan.
