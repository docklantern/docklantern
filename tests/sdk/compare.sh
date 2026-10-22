#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "${DOCKLANTERN_INTEGRATION_TEST:-}" != "1" ]]; then
  echo "Set DOCKLANTERN_INTEGRATION_TEST=1; use disposable Docker hosts only" >&2
  exit 2
fi
for tool in docker go jq; do command -v "$tool" >/dev/null || exit 2; done

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fixture="$root/tests/integration/fixtures/sample"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
project="docklantern-sdk-eval"
# Never create or manage containers in this job; only canonical config parsing.
export DOCKLANTERN_TEST_NETWORK="docklantern-sdk-evaluation-unused"
docker compose -p "$project" --project-directory "$fixture" -f "$fixture/compose.yaml" -f "$fixture/compose.prod.yaml" config --format json > "$tmp/cli.json"

cd "$root/tests/sdk"
go mod tidy
go build -trimpath -o "$tmp/sdk-eval" .
"$tmp/sdk-eval" -project-dir "$fixture" -project "$project" "$fixture/compose.yaml" "$fixture/compose.prod.yaml" > "$tmp/sdk.json"
# Compare stable semantic fields; SDK and CLI models have different JSON shapes.
jq -e --slurpfile sdk "$tmp/sdk.json" '
 .services.web.image == $sdk[0].services.web.image and
 .services.worker.image == $sdk[0].services.worker.image and
 .services.web.labels == $sdk[0].services.web.labels and
 .services.web.environment == $sdk[0].services.web.environment and
 (.services.web.volumes[0].source + ":" + .services.web.volumes[0].target) == $sdk[0].services.web.volumes[0] and
 (.services.web.networks | keys | sort) == ($sdk[0].services.web.networks | sort)
' "$tmp/cli.json" >/dev/null

# Verify service scoping at load time: unrelated worker must not be in the selected services.
"$tmp/sdk-eval" -project-dir "$fixture" -project "$project" -service web "$fixture/compose.yaml" "$fixture/compose.prod.yaml" > "$tmp/scoped.json"
jq -e '(.services | keys) == ["web"]' "$tmp/scoped.json" >/dev/null
printf 'SDK binary bytes: '; stat -c '%s' "$tmp/sdk-eval"
printf 'CLI executable bytes: '; stat -c '%s' "$(command -v docker)"
echo 'PASS: SDK load parity on image, labels, env, network, bind mount and selected service'
