#!/usr/bin/env bash
set -Eeuo pipefail

# This suite only manages unique, disposable projects. Never run it on a production host.
if [[ "${DOCKLANTERN_INTEGRATION_TEST:-}" != "1" ]]; then
  echo "Set DOCKLANTERN_INTEGRATION_TEST=1 to run disposable Docker integration tests" >&2
  exit 2
fi
for cmd in docker go jq; do command -v "$cmd" >/dev/null || { echo "Missing $cmd" >&2; exit 2; }; done
docker info >/dev/null
docker compose version >/dev/null

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fixture="$repo_root/tests/integration/fixtures/sample"
work="$(mktemp -d)"
suffix="$(tr -dc 'a-z0-9' </dev/urandom | head -c 9 || true)"
[[ ${#suffix} -eq 9 ]] || { echo "Could not create random suffix" >&2; exit 2; }
project="docklantern-ci-$suffix"
network="docklantern-ci-net-$suffix"
export DOCKLANTERN_TEST_NETWORK="$network"
project_dir="$work/project"
override_dir="$work/overrides"
mkdir -p "$project_dir"
cp -R "$fixture/." "$project_dir/"
compose=(docker compose --project-directory "$project_dir" -p "$project" -f "$project_dir/compose.yaml" -f "$project_dir/compose.prod.yaml")
cleanup() {
  local status=$?
  trap - EXIT
  "${compose[@]}" down --remove-orphans --volumes >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$work"
  exit "$status"
}
trap cleanup EXIT
docker network create --label "dev.docklantern.fixture=true" "$network" >/dev/null

cat > "$work/registration.json" <<JSON
{"name":"$project","project_dir":"$project_dir","files":["$project_dir/compose.yaml","$project_dir/compose.prod.yaml"],"services":["web"]}
JSON

go build -o "$work/docklantern" "$repo_root/cmd/docklantern"
"${compose[@]}" up -d --wait --wait-timeout 120
web_before="$( "${compose[@]}" ps -q web )"
worker_before="$( "${compose[@]}" ps -q worker )"
[[ -n "$web_before" && -n "$worker_before" ]]
[[ "$(docker inspect -f '{{.State.Running}}' "$web_before")" == "true" ]]
[[ "$(docker inspect -f '{{index .Config.Labels "traefik.enable"}}' "$web_before")" == "true" ]]
[[ "$(docker inspect -f '{{.Config.Image}}' "$web_before")" == "nginx:1.27-alpine" ]]

# Relative bind mount is resolved to this disposable host path.
resolved_source="$(docker inspect "$web_before" | jq -r '.[0].Mounts[] | select(.Destination=="/usr/share/nginx/html") | .Source')"
[[ "$resolved_source" == "$project_dir/www" ]]

# Compose discovery must identify only container identifiers, not global project operations.
"$work/docklantern" -mode discover > "$work/discovered.json"
jq -e --arg project "$project" 'any(.[]; .project==$project and .service=="web")' "$work/discovered.json" >/dev/null
jq -e --arg project "$project" 'any(.[]; .project==$project and .service=="worker")' "$work/discovered.json" >/dev/null

# Prepare only: must not mutate the running service.
"$work/docklantern" -mode prepare \
  -registration "$work/registration.json" -roots "$project_dir" \
  -service web -image nginx:1.28-alpine -output-dir "$override_dir" > "$work/prepared.txt"
prepared="$(cat "$work/prepared.txt")"
[[ -s "$prepared" ]]
[[ "$( "${compose[@]}" ps -q web )" == "$web_before" ]]
jq -e '.services.web.image=="nginx:1.28-alpine" and (.services | length)==1' "$prepared" >/dev/null
# Base config still specifies the original image; generated override is separate.
[[ "$( "${compose[@]}" config --format json | jq -r '.services.web.image' )" == "nginx:1.27-alpine" ]]

# Permission gate: no pull or recreate without explicit confirmation.
if "$work/docklantern" -mode apply -registration "$work/registration.json" \
  -roots "$project_dir" -service web -image nginx:1.28-alpine \
  -output-dir "$override_dir" >/dev/null 2>&1; then
  echo "Apply unexpectedly succeeded without confirmation" >&2; exit 1
fi
[[ "$( "${compose[@]}" ps -q web )" == "$web_before" ]]

# Failed pull: should not recreate the running service.
if "$work/docklantern" -mode apply -confirm-unsafe-poc-deploy \
  -registration "$work/registration.json" -roots "$project_dir" \
  -service web -image "nginx:tag-does-not-exist-$suffix" \
  -output-dir "$override_dir" >/dev/null 2>&1; then
  echo "Apply unexpectedly succeeded for missing image" >&2; exit 1
fi
[[ "$( "${compose[@]}" ps -q web )" == "$web_before" ]]

# Scoped deployment changes web, retains worker and Compose metadata.
"$work/docklantern" -mode apply -confirm-unsafe-poc-deploy \
  -registration "$work/registration.json" -roots "$project_dir" \
  -service web -image nginx:1.28-alpine -output-dir "$override_dir"
web_after="$( "${compose[@]}" ps -q web )"
worker_after="$( "${compose[@]}" ps -q worker )"
[[ "$web_after" != "$web_before" && -n "$web_after" ]]
[[ "$worker_after" == "$worker_before" ]]
[[ "$(docker inspect -f '{{.Config.Image}}' "$web_after")" == "nginx:1.28-alpine" ]]
[[ "$(docker inspect -f '{{index .Config.Labels "traefik.enable"}}' "$web_after")" == "true" ]]
[[ "$(docker inspect -f '{{.State.Running}}' "$web_after")" == "true" ]]
[[ "$(docker inspect "$web_after" | jq -r '.[0].Mounts[] | select(.Destination=="/usr/share/nginx/html") | .Source')" == "$project_dir/www" ]]
docker inspect "$web_after" | jq -e --arg network "$network" '.[0].NetworkSettings.Networks[$network] != null' >/dev/null
[[ "$(docker inspect "$web_after" | jq -r '.[0].Config.Env[] | select(.=="DEMO_SETTING=example")')" == "DEMO_SETTING=example" ]]

echo "PASS: disposable Compose discovery, overrides, deployment isolation, and failure gates"
