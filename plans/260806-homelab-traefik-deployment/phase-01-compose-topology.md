---
phase: 1
title: "Compose topology and safe environment contract"
status: completed
priority: P0
effort: "0.5d"
dependencies: []
---

# Phase 1: Compose topology and safe environment contract

## Requirements

- The base production file does not bind host ports.
- The homelab overlay is additive and does not duplicate image, secret, or migration definitions.
- API joins `default` and `homelab`; web joins `homelab`; migrate remains on implicit `default` only.
- Both routed services opt into Traefik and select `homelab` as the Docker network.
- API router uses `Host(`teka-api.cauchuyenlaptrinh.com`)`, entrypoint `web`, service `teka-api`, port `8080`, and a `/readyz` health check with a 10-second interval and 2-second timeout.
- Web router uses `Host(`teka-web.cauchuyenlaptrinh.com`)`, entrypoint `web`, service `teka-web`, and port `8080`.
- The overlay fixes `API_CORS_ORIGINS` and `API_STATEMENTS_PUBLIC_BASE_URL` to the web origin.
- `.env.production.example` contains no credentials and documents that the web image must be built with the accepted `VITE_API_URL`.

## Implementation

1. Remove API/web `ports` from the base production Compose file and add `expose: ["8080"]`.
2. Add the minimal homelab overlay with labels, networks, and public-origin environment overrides.
3. Declare `homelab` as an external network and retain the Compose project default network as the private application network.
4. Add a safe production env example and a narrow gitignore exception for that example only.

## Success criteria

- [x] Merged Compose output contains no published API/web ports.
- [x] Migrate has neither `homelab` membership nor Traefik labels.
- [x] API labels, networks, readiness check, and migration dependency match the contract.
- [x] No secret value is committed.
