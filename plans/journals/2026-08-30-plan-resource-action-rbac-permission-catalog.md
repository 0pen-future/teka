---
title: Plan resource-action RBAC permission catalog
date: 2026-08-30
summary: "Deep plan, red-team findings, and validation"
---

# Plan resource-action RBAC permission catalog

Created and deep-validated an eight-phase implementation plan for resource-action RBAC. Repository research confirmed a code-owned catalog and assignment-only database model. Red-team review tightened the design around alias deny canonicalization, non-grantable owner-only enforcement, direct service callers, class-role compatibility, writer-safe mixed-version deployment, per-target CAS versions, migration provenance, queued-job reauthorization, effective-impact previews, accessible conflict recovery, policy epochs, and fail-safe cleanup. The plan passed ak plan validate and was registered with ak plan use. The legacy set-active-plan session helper could not run because its bundled ck-config-utils module is missing; plan CLI registration succeeded.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
