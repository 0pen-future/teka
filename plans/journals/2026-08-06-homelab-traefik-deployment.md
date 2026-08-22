---
title: Homelab Traefik deployment
date: 2026-08-06
summary: "Implemented and verified the production Traefik overlay, safe environment contract, and operator workflow."
---

# Homelab Traefik deployment

## What happened

Executed `plans/260806-homelab-traefik-deployment`: removed production host bindings, added the external `homelab` Traefik overlay, created a fail-closed production env template, and documented the full operator workflow.

Host `go`, `golangci-lint`, and installed Vitest were unavailable. The debugger mapped those commands to repository/CI-pinned containers; API lint/tests, web tests/lint/format/typecheck, Compose assertions, and both production image builds passed.

## Decisions

- Keep migration private on the Compose default network and gate API startup on successful completion.
- Route API and web through Traefik's `web` entrypoint on external `homelab`, targeting port 8080 without host publication.
- Use deliberately short `REPLACE_ME` secret placeholders so incomplete production configuration fails validation.
- Document both local tag/push and CI `VITE_API_URL` paths for immutable web artifacts.

## Review

Mandatory review improved placeholder safety and clarified artifact publishing and service-state verification. Re-review scored 9.5/10 with all seven acceptance criteria passing and no regression or unintended contract change.

## Next steps

External DNS, Cloudflare Tunnel, Traefik, PostgreSQL provisioning, and live deployment remain operator-owned and out of scope.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
