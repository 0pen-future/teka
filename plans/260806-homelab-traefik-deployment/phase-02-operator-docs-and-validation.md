---
phase: 2
title: "Operator documentation and deployment validation"
status: completed
priority: P1
effort: "0.5d"
dependencies: [1]
---

# Phase 2: Operator documentation and deployment validation

## Requirements

- Preserve the platform-neutral image, migration, secret, and health guidance.
- State prerequisites: existing `homelab` network; Traefik Docker provider with `exposedByDefault=false`; Cloudflare Tunnel mapping both public hostnames to `http://traefik:80`; and an external PostgreSQL database reachable through the configured DSN.
- Explain immutable SHA image pinning and the web image's build-time API URL.

## Implementation

1. Document copying `.env.production.example` to ignored `.env.production` and filling its required values.
2. Document building or publishing the web image with `VITE_API_URL=https://teka-api.cauchuyenlaptrinh.com/api/v1`.
3. Document validation and startup commands using both Compose files and `--env-file .env.production`.
4. Document public readiness/web checks, Compose status, logs, and the intentional absence of application host bindings.
5. Document SHA-tag updates and rollback by changing image references and rerunning the same Compose command.

## Success criteria

- [x] An operator can validate and start the deployment without guessing file order or URLs.
- [x] Documentation separates repository-controlled configuration from out-of-scope Cloudflare, Traefik, and PostgreSQL provisioning.
- [x] Compose validation, lint, relevant tests, and production image builds pass.

## Validation commands

```bash
docker compose --env-file /tmp/teka-production.env \
  -f docker-compose.prod.yml \
  -f docker-compose.homelab.yml config
make lint
make test-api-unit
make test-web
make build-image-api
make build-image-web VITE_API_URL=https://teka-api.cauchuyenlaptrinh.com/api/v1
```

Runtime inspection of `homelab` is optional and read-only; this task does not run `docker compose up`.
