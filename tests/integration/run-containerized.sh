#!/usr/bin/env bash
set -Eeuo pipefail

# Run only on a disposable Docker-enabled host or ephemeral CI runner.
if [[ "${DOCKLANTERN_INTEGRATION_TEST:-}" != "1" ]]; then
  echo "Set DOCKLANTERN_INTEGRATION_TEST=1 on a disposable Docker host" >&2
  exit 2
fi
for cmd in docker go jq; do
  command -v "$cmd" >/dev/null || { echo "Missing $cmd" >&2; exit 2; }
done
docker info >/dev/null
docker compose version >/dev/null

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fixture="$repo_root/tests/integration/fixtures/sample"
work="$(mktemp -d)"
suffix="$(tr -dc 'a-z0-9' </dev/urandom | head -c 10 || true)"
if [[ ${#suffix} -ne 10 ]]; then
  echo "Failed to create random fixture suffix" >&2
  exit 2
fi
project="docklantern-container-$suffix"
network="docklantern-container-net-$suffix"
image="docklantern-poc-test:$suffix"
export DOCKLANTERN_TEST_NETWORK="$network"
project_dir="$work/project"
override_dir="$work/overrides"
mkdir -p "$project_dir" "$override_dir"
cp -R "$fixture/." "$project_dir/"

compose=(docker compose --project-directory "$project_dir" -p "$project" -f "$project_dir/compose.yaml" -f "$project_dir/compose.prod.yaml")
cleanup() {
  status=$?
  trap - EXIT
  "${compose[@]}" down --remove-orphans --volumes >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  docker image rm "$image" >/dev/null 2>&1 || true
  rm -rf "$work"
  exit "$status"
}
trap cleanup EXIT

docker network create --label dev.docklantern.fixture=true "$network" >/dev/null
cat > "$work/registration.json" <<JSON
{"name":"$project","project_dir":"$project_dir","files":["$project_dir/compose.yaml","$project_dir/compose.prod.yaml"],"services":["web"]}
JSON

docker build -f "$repo_root/Dockerfile.poc" -t "$image" "$repo_root"
# The key path-parity guarantee: both Compose source and generated overrides
# have IDENTICAL absolute paths from the host and the executor container.
# Test image is deliberately socket-privileged; never expose it publicly.
runner=(docker run --rm --network none
  --mount "type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock"
  --mount "type=bind,src=$project_dir,dst=$project_dir,readonly"
  --mount "type=bind,src=$work,dst=$work"
  -e DOCKLANTERN_TEST_NETWORK="$network"
  "$image")

# Precondition: Compose CLI plugin exists inside the executor image.
docker run --rm --entrypoint docker "$image" compose version
"${compose[@]}" up -d --wait --wait-timeout 120
web_before="$("${compose[@]}" ps -q web)"
worker_before="$("${compose[@]}" ps -q worker)"
[[ -n "$web_before" && -n "$worker_before" ]]

# Docker daemon access and label discovery FROM INSIDE the updater container.
"${runner[@]}" -mode discover > "$work/discovered.json"
jq -e --arg project "$project" 'any(.[]; .project==$project and .service=="web")' "$work/discovered.json" >/dev/null
jq -e --arg project "$project" 'any(.[]; .project==$project and .service=="worker")' "$work/discovered.json" >/dev/null

# Load two original source files from a read-only identical-path mount;
# generate an image-only override in the writable shared fixture directory.
"${runner[@]}" -mode prepare -registration "$work/registration.json" \
  -roots "$project_dir" -service web -image nginx:1.28-alpine \
  -output-dir "$override_dir" > "$work/prepared.txt"
prepared="$(cat "$work/prepared.txt")"
[[ "$prepared" == "$override_dir/"* && -s "$prepared" ]]
jq -e '.services.web.image=="nginx:1.28-alpine" and (.services | length)==1' "$prepared" >/dev/null
[[ "$("${compose[@]}" ps -q web)" == "$web_before" ]]
[[ "$("${compose[@]}" config --format json | jq -r '.services.web.image')" == "nginx:1.27-alpine" ]]

# Reject invalid image without touching services.
if "${runner[@]}" -mode apply -confirm-unsafe-poc-deploy \
  -registration "$work/registration.json" -roots "$project_dir" \
  -service web -image "nginx:tag-does-not-exist-$suffix" \
  -output-dir "$override_dir" >/dev/null 2>&1; then
  echo "Invalid image unexpectedly deployed" >&2
  exit 1
fi
[[ "$("${compose[@]}" ps -q web)" == "$web_before" ]]

# Permission gate: no deployment without the explicit fixture-only flag.
if "${runner[@]}" -mode apply -registration "$work/registration.json" \
  -roots "$project_dir" -service web -image nginx:1.28-alpine \
  -output-dir "$override_dir" >/dev/null 2>&1; then
  echo "Deployment gate unexpectedly bypassed" >&2
  exit 1
fi
[[ "$("${compose[@]}" ps -q web)" == "$web_before" ]]

# Real, scoped update FROM INSIDE DockLantern.
"${runner[@]}" -mode apply -confirm-unsafe-poc-deploy \
  -registration "$work/registration.json" -roots "$project_dir" \
  -service web -image nginx:1.28-alpine -output-dir "$override_dir"

web_after="$("${compose[@]}" ps -q web)"
worker_after="$("${compose[@]}" ps -q worker)"
[[ -n "$web_after" && "$web_after" != "$web_before" ]]
[[ "$worker_after" == "$worker_before" ]]
[[ "$(docker inspect -f '{{.Config.Image}}' "$web_after")" == "nginx:1.28-alpine" ]]
[[ "$(docker inspect -f '{{index .Config.Labels "traefik.enable"}}' "$web_after")" == "true" ]]
[[ "$(docker inspect -f '{{.State.Running}}' "$web_after")" == "true" ]]
docker inspect "$web_after" | jq -e --arg net "$network" '.[0].NetworkSettings.Networks[$net] != null' >/dev/null
mount_source="$(docker inspect "$web_after" | jq -r '.[0].Mounts[] | select(.Destination=="/usr/share/nginx/html") | .Source')"
[[ "$mount_source" == "$project_dir/www" ]]
docker inspect "$web_after" | jq -e 'any(.[0].Config.Env[]; .=="DEMO_SETTING=example")' >/dev/null
[[ "$(cat "$project_dir/compose.yaml" | grep -c 'image: nginx:1.27-alpine')" == "1" ]]
echo "PASS: containerized Compose execution, path parity, preserved labels/network/mount/env, scoped replacement"
