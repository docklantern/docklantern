#!/usr/bin/env bash
# Disposable, real-Docker beta.3 release gate: onboarding AFTER DockLantern
# starts, through the restricted GET-only Docker proxy, with no source mounts.
set -Eeuo pipefail
if [[ "${DOCKLANTERN_INTEGRATION_TEST:-}" != "1" ]]; then
  echo "Only run on an explicitly disposable CI Docker host" >&2
  exit 2
fi
for cmd in docker jq mktemp; do command -v "$cmd" >/dev/null || exit 2; done
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
scratch="$(mktemp -d)"
suffix="${RANDOM}${RANDOM}"
project="docklantern-beta3-${suffix}"
bridge="docklantern-beta3-proxy-${suffix}"
egress="docklantern-beta3-egress-${suffix}"
proxy="docklantern-beta3-socket-${suffix}"
lantern="docklantern-beta3-app-${suffix}"
appImage="docklantern-beta3-ci:${suffix}"
proxyImage="docklantern-beta3-proxy:${suffix}"
volume="docklantern-beta3-db-${suffix}"
compose="$scratch/compose.yaml"
labels="$scratch/compose.monitor.yaml"
cleanup(){
 status=$?
 if (( status != 0 )); then
  docker logs "$lantern" --tail 100 >&2 || true
  docker logs "$proxy" --tail 50 >&2 || true
  docker compose -p "$project" -f "$compose" -f "$labels" ps -a >&2 || true
 fi
 docker compose -p "$project" -f "$compose" -f "$labels" down --remove-orphans >/dev/null 2>&1 || true
 docker rm -f "$lantern" "$proxy" >/dev/null 2>&1 || true
 docker volume rm "$volume" >/dev/null 2>&1 || true
 docker network rm "$bridge" "$egress" >/dev/null 2>&1 || true
 docker image rm "$appImage" "$proxyImage" >/dev/null 2>&1 || true
 rm -rf "$scratch"
}
trap cleanup EXIT

cat > "$compose" <<'YAML'
services:
  web:
    image: busybox:1.36
    command: ["sh", "-c", "sleep 600"]
YAML
# Keep a separate overlay so the first instance has NO DockLantern labels.
cat > "$labels" <<'YAML'
services:
  web:
    labels:
      com.docklantern.monitor: "true"
      com.docklantern.interval: "10m"
YAML

docker pull busybox:1.36 >/dev/null
docker build -q -t "$appImage" "$root" >/dev/null
docker build -q -f "$root/deploy/docker-socket-proxy/Dockerfile" -t "$proxyImage" "$root" >/dev/null
docker network create --internal "$bridge" >/dev/null
docker network create "$egress" >/dev/null
docker volume create "$volume" >/dev/null
docker compose -p "$project" -f "$compose" up -d --pull never
unlabeledID="$(docker compose -p "$project" -f "$compose" ps -q web)"
[[ -n "$unlabeledID" ]]
docker run -d --name "$proxy" --network "$bridge" --network-alias docker-socket-proxy \
 --mount type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock,readonly \
 -e CONTAINERS=1 -e IMAGES=1 -e POST=0 -e EVENTS=0 \
 -e NETWORKS=0 -e INFO=0 -e SERVICES=0 -e VOLUMES=0 \
 "$proxyImage" >/dev/null

# No bind-mounted project directory, no -roots flag, no host-published port,
# and no exposed Docker socket in the DockLantern application container.
docker run -d --name "$lantern" --network "$bridge" \
 -e DOCKLANTERN_DOCKER_PROXY_URL=http://docker-socket-proxy:2375 \
 -e DOCKLANTERN_ADMIN_USERNAME=ci-admin \
 -e DOCKLANTERN_ADMIN_PASSWORD=disposable-ci-password-not-for-production \
 -e DOCKLANTERN_ALLOW_PROXY=1 \
 -e DOCKLANTERN_PUBLIC_ORIGIN=https://docklantern.example.test \
 --mount type=volume,source="$volume",target=/data \
 "$appImage" -mode serve -db /data/docklantern.db -listen 0.0.0.0:8787 \
 -run-monitor -scan-every 60s >/dev/null
docker network connect "$egress" "$lantern"
for i in $(seq 1 45); do
 if docker exec "$lantern" wget -q -O - http://127.0.0.1:8787/readyz 2>/dev/null | grep -q '"status":"ready"'; then
  break
 fi
 sleep 1
done
docker exec "$lantern" wget -q -O - http://127.0.0.1:8787/readyz | grep -q '"status":"ready"'
lanternID="$(docker inspect "$lantern" --format '{{.Id}}')"
lanternStarted="$(docker inspect "$lantern" --format '{{.State.StartedAt}}')"
docker exec "$lantern" docklantern-inventory -mode projects -db /data/docklantern.db > "$scratch/projects.json"
jq -e 'length==0' "$scratch/projects.json" >/dev/null
docker exec "$lantern" docklantern-inventory -mode policies -db /data/docklantern.db > "$scratch/policies.json"
jq -e 'length==0' "$scratch/policies.json" >/dev/null

# Redeploy ONLY the monitored application after DockLantern is already healthy.
# No change to DockLantern flags, volumes, networks, or configuration.
docker compose -p "$project" -f "$compose" -f "$labels" up -d --pull never --no-deps web
labeledID="$(docker compose -p "$project" -f "$compose" -f "$labels" ps -q web)"
[[ -n "$labeledID" && "$labeledID" != "$unlabeledID" ]]
labeledStarted="$(docker inspect "$labeledID" --format '{{.State.StartedAt}}')"
labeledImageID="$(docker inspect "$labeledID" --format '{{.Image}}')"

# A successful next PERIODIC SCAN (not a manual -mode scan) must enroll it.
found=0
for i in $(seq 1 95); do
 docker exec "$lantern" docklantern-inventory -mode policies -db /data/docklantern.db > "$scratch/policies.json"
 if jq -e --arg project "$project" '
   length==1 and .[0].project==$project and .[0].service=="web"
   and .[0].interval_seconds==600
   and .[0].platform=="linux/amd64"
   and .[0].paused==false' "$scratch/policies.json" >/dev/null; then
   found=1; break
 fi
 sleep 1
done
if [[ "$found" != "1" ]];then echo "Timed out waiting for automatic label enrollment" >&2;exit 1;fi

# Monitoring is enrollment, not Compose registration or Docker mutation.
docker exec "$lantern" docklantern-inventory -mode projects -db /data/docklantern.db > "$scratch/projects.json"
jq -e 'length==0' "$scratch/projects.json" >/dev/null
[[ "$(docker inspect "$lantern" --format '{{.Id}}')" == "$lanternID" ]]
[[ "$(docker inspect "$lantern" --format '{{.State.StartedAt}}')" == "$lanternStarted" ]]
[[ "$(docker inspect "$labeledID" --format '{{.State.StartedAt}}')" == "$labeledStarted" ]]
[[ "$(docker inspect "$labeledID" --format '{{.Image}}')" == "$labeledImageID" ]]
[[ "$(docker inspect "$labeledID" --format '{{.State.Running}}')" == "true" ]]

# Strictly deny Docker mutation methods through the socket proxy.
result="$(docker run --rm --network "$bridge" --entrypoint wget busybox:1.36 \
 -q -S -O /dev/null --post-data=x http://docker-socket-proxy:2375/containers/create 2>&1 || true)"
if ! grep -q '403 Forbidden' <<< "$result"; then
 echo "Docker proxy unexpectedly permitted container creation" >&2
 exit 1
fi
echo "PASS beta.3: zero-DockLantern-change label enrollment, intact container IDs, no registration, GET-only proxy"
