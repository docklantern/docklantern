#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "${DOCKLANTERN_INTEGRATION_TEST:-}" != "1" ]];then
  echo "Disposable CI environment only" >&2;exit 2
fi
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fixture="$root/tests/integration/monitoring-flow/compose.yaml"
cleanup(){ docker compose -p docklantern-ci-flow -f "$fixture" down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker compose -p docklantern-ci-flow -f "$fixture" up -d --pull always
(
 cd "$root"
 go test -count=1 -race -run '^TestDisposableComposePipeline$' ./internal/notify
)
