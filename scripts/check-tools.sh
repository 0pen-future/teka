#!/usr/bin/env bash
# Verify the local toolchain needed for development.
set -euo pipefail

missing=0

need() {
  local tool="$1" hint="$2"
  if command -v "$tool" >/dev/null 2>&1; then
    printf '  ok  %s\n' "$tool"
  else
    printf 'MISS  %s  (%s)\n' "$tool" "$hint"
    missing=1
  fi
}

echo "Checking required tools:"
need git "https://git-scm.com"
need go "https://go.dev/dl (1.22+)"
need node "https://nodejs.org (20+)"
need npm "ships with node"
need docker "https://docs.docker.com/get-docker"
need make "xcode-select --install / build-essential"

if ! docker compose version >/dev/null 2>&1; then
  echo 'MISS  docker compose plugin  (Docker Desktop or docker-compose-plugin)'
  missing=1
else
  printf '  ok  docker compose\n'
fi

echo
echo "Optional (host-mode development):"
command -v air >/dev/null 2>&1 && echo '  ok  air' || echo '  --  air (go install github.com/air-verse/air@latest)'
command -v golangci-lint >/dev/null 2>&1 && echo '  ok  golangci-lint' || echo '  --  golangci-lint (https://golangci-lint.run/welcome/install)'

if [ "$missing" -ne 0 ]; then
  echo
  echo "Install the missing required tools above, then re-run: make setup" >&2
  exit 1
fi
