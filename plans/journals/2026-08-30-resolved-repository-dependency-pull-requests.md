---
title: Resolved repository dependency pull requests
date: 2026-08-30
summary: Integrated and verified 12 dependency PRs on master; two resolved PR records remain open due GitHub permissions.
---

# Resolved repository dependency pull requests

## What happened

Reviewed every open dependency PR against current `master`. Diagnosed PR #27 failures as stricter optional-chain lint plus Sonner 2.0.8 toast-state replay, PR #31 as incompatible TypeScript 7 peer constraints, and PRs #28-#30 as exposing GO-2026-6253 in `moby/go-archive` rather than causing it.

Integrated one commit per PR for #25-#33, then handled replacement PRs #36-#38 opened by Dependabot after the first push. Updated web test cleanup, the vulnerable Go archive dependency, workflow actions, container images, and compatible web dependencies. Deferred TypeScript 7 through scoped Dependabot policy.

## Verification

API: 46 packages passed; pinned govulncheck reported zero reachable vulnerabilities; API CI, registry login, build, and publish passed. Web: lint, format, typecheck, 449 tests with 3 skipped, coverage, build, audit, Docker build, registry publish, and Playwright E2E passed. Independent review found no issues. `master` and `origin/master` matched after each push.

## Decision

Do not reintroduce TypeScript 7 until typescript-eslint supports its compiler API. Keep the Sonner global-state cleanup in the shared test setup. Treat #36 and #37 as code-resolved even though their GitHub PR records remain open.

## Next steps

A repository administrator must close PRs #36 and #37. The current `dev-fng` credential can comment but GitHub denies ClosePullRequest and REST update access. Review the remaining high-severity Dependabot alert reported by GitHub separately.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
