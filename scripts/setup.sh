#!/usr/bin/env bash
# One-shot dev environment bootstrap. Equivalent to `make setup`.
set -euo pipefail
cd "$(dirname "$0")/.."

./scripts/check-tools.sh
[ -f .env ] || { cp .env.example .env && echo "Created .env from .env.example"; }
if [ -f package-lock.json ]; then npm ci --silent; else npm install --silent; fi
(cd apps/web && if [ -f package-lock.json ]; then npm ci --silent; else npm install --silent; fi)
npx lefthook install
echo "Setup complete. Run 'make dev' to start the stack."
