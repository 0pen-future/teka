#!/usr/bin/env bash
# Wait for a TCP endpoint to accept connections: wait-for.sh host:port [timeout_seconds]
# Host-machine helper (uses bash /dev/tcp); not for Alpine/busybox containers.
set -euo pipefail

target="${1:?usage: wait-for.sh host:port [timeout_seconds]}"
timeout="${2:-30}"

case "$target" in
  *:*) host="${target%%:*}"; port="${target##*:}" ;;
  *) echo "invalid target '${target}': expected host:port" >&2; exit 2 ;;
esac

for _ in $(seq 1 "$timeout"); do
  if (exec 3<>"/dev/tcp/${host}/${port}") 2>/dev/null; then
    echo "${target} is up"
    exit 0
  fi
  sleep 1
done

echo "timed out after ${timeout}s waiting for ${target}" >&2
exit 1
