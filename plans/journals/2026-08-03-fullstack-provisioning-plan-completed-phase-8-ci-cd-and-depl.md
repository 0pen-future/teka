---
title: Fullstack provisioning plan completed — Phase 8 CI/CD and deployment
date: 2026-08-03
summary: "Final phase shipped as 1d9b800: path-filtered GitHub Actions, security scanning, digest-pinned production images, deployment docs; all 8 plan phases complete"
---

# Fullstack provisioning plan completed — Phase 8 CI/CD and deployment

## What happened

Phase 8 closed out plan `260803-1552-fullstack-project-provisioning` (commit `1d9b800`, the eighth phase commit on main). Delivered: path-filtered `api-ci.yml` / `web-ci.yml` (e2e quarantined to main pushes + nightly), weekly `security.yml` (govulncheck pinned v1.1.4, audit-ci, Trivy matrix scan with SARIF), Dependabot for gomod/npm/actions/docker, digest-pinned multi-stage production images (distroless API 38MB with embedded migrations and `-ldflags -X main.version` stamping; unprivileged-nginx web 55MB), `docker-compose.prod.yml` reference topology, route-level `route.lazy` code splitting plus bundle analyzer, `make build` now producing both images, and a full `docs/deployment.md`.

Everything runnable locally was verified live: both images smoke-tested against a throwaway postgres (`migrate up`, `/healthz` 200, `--version` prints the git sha, non-root uids 65532/101, SPA fallback, immutable asset caching, `/api/` 404 guard), e2e 6/6 against the dev stack proving lazy routes, actionlint clean, API coverage 71.4% against the 60% floor.

## Key findings caught by review

The code-reviewer pass (DONE_WITH_CONCERNS, `plans/reports/phase-08-code-review.md`) caught three High findings that local tooling could not: `aquasecurity/trivy-action@0.28.0` does not exist as a tag (actionlint never resolves remote refs — fixed to `@v0.36.0`), Trivy's sarif format silently discards the severity filter (fixed with `limit-severities-for-sarif` + `ignore-unfixed`), and the deployment doc pointed readiness probes at `/healthz`, which never touches the DB (fixed: `/readyz` is the readiness endpoint). Also closed a finding that had survived since Phase 5: the web Dockerfile now fails the build when `VITE_API_URL` is missing instead of shipping a white-screen bundle.

Security sweep found two real HIGHs: quic-go GO-2026-5676 (bumped v0.59.0→v0.59.1, govulncheck now clean) and react-router GHSA-qwww-vcr4-c8h2 (RSC-mode CSRF — app uses plain SPA data mode, no 7.x patch exists, allowlisted in audit-ci with the v8 major upgrade deferred as an explicit decision).

## Decisions

- nginx-unprivileged:1.29 over the plan's nginx:alpine so the web container runs non-root (accepted at the commit gate).
- `make build` semantics changed to Docker images; host targets preserved as `build-api`/`build-web`.
- All four base images digest-pinned; Dependabot docker ecosystem owns the bumps.

## Open items

- GitHub-dependent success criteria (Actions runs, GHCR push, Dependabot, SARIF tab) are implemented and locally validated but need a remote to verify live; the plan-level checkbox stays intentionally open.
- Deferred: react-router v8 upgrade (drops the audit allowlist), concrete deployment target, web coverage floor, SARIF-on-private-repo needs GHAS.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
