#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "${DOCKLANTERN_INTEGRATION_TEST:-}" != "1" ]];then
  echo "Only run on a disposable Docker host" >&2;exit 2
fi
for cmd in docker jq;do command -v "$cmd" >/dev/null || exit 2;done
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
suffix="$RANDOM$RANDOM"
network="docklantern-discovery-$suffix"
proxy="docklantern-proxy-$suffix"
fixture="docklantern-fixture-$suffix"
image="docklantern-test-discovery:$suffix"
cleanup(){
 docker rm -f "$fixture" "$proxy" >/dev/null 2>&1 || true
 docker network rm "$network" >/dev/null 2>&1 || true
 docker image rm "$image" "docklantern-proxy-patched:$suffix" >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker network create --internal "$network" >/dev/null
# Pull images before creating the isolated network. Only the proxy has the socket.
docker build -f "$repo/deploy/docker-socket-proxy/Dockerfile" -t "docklantern-proxy-patched:$suffix" "$repo" >/dev/null
docker pull busybox:1.36 >/dev/null
docker build -f "$repo/Dockerfile" -t "$image" "$repo" >/dev/null
docker run -d --name "$fixture" \
 --label com.docker.compose.project=docklantern-ci-discovery \
 --label com.docker.compose.service=web \
 busybox:1.36 sh -c 'sleep 120' >/dev/null
docker run -d --name "$proxy" --network "$network" --network-alias docker-socket-proxy \
 --mount type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock,readonly \
 -e CONTAINERS=1 -e POST=0 -e EVENTS=0 -e IMAGES=1 \
 -e NETWORKS=0 -e INFO=0 -e SERVICES=0 -e VOLUMES=0 \
 "docklantern-proxy-patched:$suffix" >/dev/null

for i in $(seq 1 20);do
 if docker run --rm --network "$network" \
  -e DOCKLANTERN_DOCKER_PROXY_URL=http://docker-socket-proxy:2375 \
  "$image" -mode discover > /tmp/docklantern-proxy-result-"$suffix".json 2>/dev/null;then break;fi
 sleep 1
done
result=/tmp/docklantern-proxy-result-"$suffix".json
trap 'rm -f /tmp/docklantern-proxy-result-'"$suffix"'.json;cleanup' EXIT
jq -e 'any(.[]; .project=="docklantern-ci-discovery" and .service=="web" and .state=="running" and .image_os=="linux" and .image_architecture!="")' "$result" >/dev/null

# Docker proxy must forbid every modification even when container GET is granted.
code="$(docker run --rm --network "$network" --entrypoint wget busybox:1.36 \
  -q -S -O /dev/null --post-data=x http://docker-socket-proxy:2375/containers/create 2>&1 || true)"
if ! grep -q '403 Forbidden' <<<"$code";then
 echo "Docker proxy mutation not blocked" >&2;exit 1
fi
echo "PASS: read-only socket proxy discovery, Compose labels, mutation rejection"
