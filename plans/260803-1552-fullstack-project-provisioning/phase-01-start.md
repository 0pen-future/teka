---
phase: 1
title: "Phase 1: Repository Foundation"
status: todo
priority: P1
effort: "4h"
dependencies: []
---

# Phase 1: Repository Foundation

## Overview

Create the monorepo skeleton: top-level layout, Makefile task surface, environment examples, git hygiene, hooks with conventional commits, and documentation stubs. Everything later phases hang off.

## Requirements

- [x] Top-level directories exist per the tree in `plan.md` (`apps/api`, `apps/web`, `infrastructure`, `scripts`, `docs`, `.github`)
- [x] Root `Makefile` provides the agreed command surface (stubs may delegate to per-app targets added in later phases)
- [x] Git hooks enforce format/lint on commit and conventional commits on commit-msg
- [x] `.env.example` documents every variable the stack will use
- [x] `README.md` explains the repo layout and quickstart

## Architecture

- Root `Makefile` is the single orchestrator; it delegates into `apps/api` and `apps/web`. No workspace tooling (Nx/Turbo) — YAGNI.
- Hooks via **lefthook** (single binary, polyglot) + **commitlint** (`@commitlint/config-conventional`). Node is only required at repo root for commitlint; keep root `package.json` minimal (commitlint + lefthook devDependencies).
- `.editorconfig` covers Go (tabs), TS/JSON/YAML (2 spaces), Makefile (tabs).

## Related Code Files

- Create: `Makefile`, `README.md`, `.gitignore`, `.editorconfig`, `.env.example`
- Create: `lefthook.yml`, `commitlint.config.mjs`, `package.json` (root, hooks-only)
- Create: `scripts/setup.sh`, `scripts/check-tools.sh`, `scripts/wait-for.sh`
- Create: `docs/architecture.md`, `docs/api-guidelines.md`, `docs/frontend-guidelines.md`, `docs/local-development.md`, `docs/deployment.md` (stubs with owned-topics headers)
- Create: `.github/` directory (workflows land in Phase 8)

## Implementation Steps

1. Create the directory tree with `.gitkeep` only where a directory must exist empty; prefer creating real files.
2. Write `.gitignore`: Go build artifacts, `node_modules`, `dist`, `.env*` (except `.env.example`), IDE dirs, `apps/api/docs/` generated swagger, coverage output, `.air` tmp.
3. Write `.env.example` with commented sections:
   - `POSTGRES_USER/PASSWORD/DB/PORT`
   - `API_ENV=development`, `API_HTTP_PORT=8080`, `API_DATABASE_URL`, `API_JWT_SECRET`, `API_JWT_ACCESS_TTL=15m`, `API_JWT_REFRESH_TTL=720h`, `API_LOG_LEVEL=debug`, `API_CORS_ORIGINS=http://localhost:5173`
   - `VITE_API_URL=http://localhost:8080/api/v1`
4. Write root `Makefile` with targets (later phases fill implementations):
   ```make
   setup dev dev-down api-dev web-dev test test-api test-web lint lint-api lint-web \
   fmt migrate-up migrate-down migrate-status seed build build-api build-web hooks
   ```
   - `setup`: `scripts/check-tools.sh` (go, node, docker, docker compose, make), copy `.env.example`→`.env` if absent, install root hook deps, `lefthook install`.
   - `dev`: `docker compose up --build`; `dev-down`: `docker compose down`.
5. Configure `lefthook.yml`:
   - `pre-commit`: `gofmt`/`golangci-lint run --fix` on staged Go files (skip until Phase 2 lands, guard with dir existence); `eslint --fix` + `prettier --write` on staged web files.
   - `commit-msg`: `commitlint --edit`.
6. Write `README.md`: what the project is, prerequisites, `make setup && make dev` quickstart, layout table, link to `docs/`.
7. Write doc stubs, each with an "Owns" line so later phases know where content lands (per `documentation-management.md`, smallest owning surface).
8. Commit: `chore: scaffold monorepo foundation`.

## Todo

- [x] Directory tree + gitkeep/real files
- [x] `.gitignore`, `.editorconfig`, `.env.example`
- [x] Root `Makefile` with full target surface
- [x] lefthook + commitlint wired, `make hooks` installs them
- [x] `scripts/setup.sh`, `check-tools.sh`, `wait-for.sh`
- [x] `README.md` + docs stubs

## Success Criteria

- [x] Fresh clone: `make setup` completes without error on macOS/Linux
- [x] A commit with message `bad message` is rejected; `feat: x` passes
- [x] `make` (help target) lists all commands with one-line descriptions

## Risk Assessment

- **Hook friction** — hooks that fail on missing toolchains block commits. Mitigation: lefthook commands guard on tool presence and staged-file globs; `LEFTHOOK=0` documented escape hatch.
- **Makefile drift** — stub targets that error confusingly. Mitigation: stubs print "provisioned in phase N" until implemented.
