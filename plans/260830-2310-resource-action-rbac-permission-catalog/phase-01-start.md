---
title: "Phase 1: Inventory and compatibility contract"
status: done
estimate: "1.5 days"
dependsOn: []
---
# Phase 1: Inventory and compatibility contract

Freeze the authorization surface before enforcement changes.

## Tasks
- [x] Enumerate `apps/api/internal/features/*/routes.go`; classify every route as public, authenticated-self, owner-only, or permission.
- [x] Record method/path, canonical key, legacy aliases, tenant lookup, object visibility, and extra capability predicate.
- [x] Define resources/CRUD keys; omit meaningless actions and document irregular mappings.
- [x] Freeze the namespace convention: catalog keys use only canonical CRUD verbs, `<resource>.view_all` scope keys, and named specials; no catalog key may equal a class-staff capability string.
- [x] Inventory every resource with assigned-vs-center-wide visibility and freeze its `<resource>.view_all` scope key replacing `data.view_center_wide`.
- [x] Give every multi-resource aggregate endpoint (dashboard, exports, pickers) one explicit policy key instead of composing multiple `view_all` keys.
<!-- Updated: Validation Session 1 - namespace convention and per-resource scope-key decomposition -->

- [x] Inventory domain commands; assign stable special keys, risk, grantability, and audit event.
- [x] Freeze explicit old-to-new mappings for grants and denies; forbid wildcard/prefix aliases.
- [x] Reproduce current access as canonical built-in defaults and define custom-role expansion.
- [x] Freeze non-grantable administration, staffing/handoff, sensitive review, and unsafe financial actions.
- [x] Document mixed versions and review every high-risk/non-grantable item.

## Acceptance and verification
- [x] Route count matches registration; each key has one definition and each legacy key a disposition.
- [x] Owner/admin/staff/custom-role fixtures match current effective access, including a class-staff assignment dimension (active and ended stints) per resource — baseline visibility for attendance, grading, sessions, and enrollments changed to assignment-based scoping in plan 260830-0938 this week.
<!-- Updated: Validation Session 1 (post-validation kongming review) - aggregate-endpoint keys and assignment-dimension fixtures -->
- [x] Trace one CRUD, special, owner-only, self, and public-token flow through route, service, and repository.
