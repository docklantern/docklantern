#!/usr/bin/env bash
# B6-05 destructive fixture: ISOLATED GitHub Actions runner ONLY.
# This intentionally operates on disposable Docker resources. It is not
# the production executor and never loads DockLantern host registrations.
set -euo pipefail
if [[ "${DOCKLANTERN_DISPOSABLE_APPLY_TEST:-}" != "1" ||
      "${GITHUB_ACTIONS:-}" != "true" ||
      "${RUNNER_ENVIRONMENT:-}" != "github-hosted" ]]; then
  echo "refusing disposable Docker mutation outside approved CI" >&2
  exit 3
fi
command -v docker >/dev/null
docker info >/dev/null
ROOT="$(mktemp -d)"
NONCE="b6apply-${GITHUB_RUN_ID:?}-${GITHUB_RUN_ATTEMPT:?}"
PROJECT="${NONCE//[^a-zA-Z0-9-]/}"
REGISTRY="${PROJECT}-registry"
NET="${PROJECT}-net"
cleanup(){
  docker compose -p "$PROJECT" --project-directory "$ROOT" -f "$ROOT/compose.yaml" down --remove-orphans >/dev/null 2>&1 || true
  docker rm -f "$REGISTRY" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$ROOT"
}
trap cleanup EXIT
mkdir -p "$ROOT/first" "$ROOT/second"
docker network create "$NET" >/dev/null
# Local registry exists only on the disposable runner; no cloud credentials.
docker run -d --name "$REGISTRY" -p 127.0.0.1:0:5000 registry:2 >/dev/null
PORT="$(docker inspect -f '{{(index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort}}' "$REGISTRY")"
[[ "$PORT" =~ ^[0-9]+$ ]]
REG="localhost:${PORT}"
for i in {1..30}; do
  if curl --fail --silent "http://${REG}/v2/" >/dev/null; then break; fi
  sleep 1
done
curl --fail --silent "http://${REG}/v2/" >/dev/null
# BuildKit may not retain the FROM image in the local image store.
docker pull busybox:1.36 >/dev/null
cat >"$ROOT/first/Dockerfile" <<'DOCKERFILE'
FROM busybox:1.36
LABEL docklantern.fixture=old
CMD ["sh","-c","sleep 600"]
DOCKERFILE
cat >"$ROOT/second/Dockerfile" <<'DOCKERFILE'
FROM busybox:1.36
LABEL docklantern.fixture=new
CMD ["sh","-c","sleep 600"]
DOCKERFILE
docker build -q -t "${REG}/fixture/target:old" "$ROOT/first" >/dev/null
docker build -q -t "${REG}/fixture/target:new" "$ROOT/second" >/dev/null
docker push "${REG}/fixture/target:old" >/dev/null
docker push "${REG}/fixture/target:new" >/dev/null
NEW_DIGEST="$(docker image inspect "${REG}/fixture/target:new" --format '{{index .RepoDigests 0}}')"
[[ "$NEW_DIGEST" == "${REG}/fixture/target@sha256:"* ]]
cat >"$ROOT/compose.yaml" <<COMPOSE
services:
  target:
    image: ${REG}/fixture/target:old
    pull_policy: never
    labels:
      docklantern.fixture: target
    networks:
      - stable
  sibling:
    image: busybox:1.36
    pull_policy: never
    command: ["sh","-c","sleep 600"]
    labels:
      docklantern.fixture: sibling
    networks:
      - stable
networks:
  stable:
    name: ${NET}
    external: true
COMPOSE
dc(){ docker compose --project-directory "$ROOT" -p "$PROJECT" -f "$ROOT/compose.yaml" "$@"; }
dc up --no-build -d
TARGET_BEFORE="$(dc ps -q target)"
SIBLING_BEFORE="$(dc ps -q sibling)"
[[ -n "$TARGET_BEFORE" && -n "$SIBLING_BEFORE" ]]
SIBLING_INVARIANT="$(docker inspect "$SIBLING_BEFORE" --format '{{.Id}}|{{.Image}}|{{json .Config.Labels}}|{{json .NetworkSettings.Networks}}|{{json .Mounts}}')"
TARGET_IMAGE_BEFORE="$(docker inspect "$TARGET_BEFORE" --format '{{.Image}}')"
# A missing immutable image with --pull never is a pre-apply denial and
# must not disturb the previously running service or its sibling.
MISSING="sha256:$(printf '0%.0s' {1..64})"
if printf 'services:\n  target:\n    image: %s/fixture/target@%s\n' "$REG" "$MISSING" |
  docker compose --project-directory "$ROOT" -p "$PROJECT" \
    -f "$ROOT/compose.yaml" -f - up --no-deps --no-build --pull never \
    --force-recreate -d target >"$ROOT/expected-denial.log" 2>&1; then
  echo "FAIL: nonexistent immutable candidate unexpectedly applied" >&2
  exit 1
fi
[[ "$(dc ps -q target)" == "$TARGET_BEFORE" ]]
[[ "$(dc ps -q sibling)" == "$SIBLING_BEFORE" ]]
# Immutable candidate override via stdin, never changes user's Compose YAML.
# --no-deps excludes Compose dependencies; --no-build prohibits builds;
# --pull never relies only on the separately pre-pulled immutable reference.
printf 'services:\n  target:\n    image: %s\n' "$NEW_DIGEST" |
  docker compose --project-directory "$ROOT" -p "$PROJECT" \
    -f "$ROOT/compose.yaml" -f - up --no-deps --no-build --pull never \
    --force-recreate -d target
TARGET_AFTER="$(dc ps -q target)"
SIBLING_AFTER="$(dc ps -q sibling)"
[[ -n "$TARGET_AFTER" && "$TARGET_AFTER" != "$TARGET_BEFORE" ]]
[[ "$SIBLING_AFTER" == "$SIBLING_BEFORE" ]]
SIBLING_AFTER_INVARIANT="$(docker inspect "$SIBLING_AFTER" --format '{{.Id}}|{{.Image}}|{{json .Config.Labels}}|{{json .NetworkSettings.Networks}}|{{json .Mounts}}')"
[[ "$SIBLING_INVARIANT" == "$SIBLING_AFTER_INVARIANT" ]]
TARGET_IMAGE_AFTER="$(docker inspect "$TARGET_AFTER" --format '{{.Image}}')"
[[ "$TARGET_IMAGE_AFTER" != "$TARGET_IMAGE_BEFORE" ]]
[[ "$(docker inspect "$TARGET_AFTER" --format '{{.State.Running}}')" == "true" ]]
[[ "$(docker inspect "$TARGET_AFTER" --format '{{index .Config.Labels "docklantern.fixture"}}')" == "target" ]]
[[ "$(docker image inspect "$TARGET_IMAGE_AFTER" --format '{{index .Config.Labels "docklantern.fixture"}}')" == "new" ]]
# Ordinary Compose YAML is byte-for-byte unmodified by image-only replacement.
grep -q 'image: .*target:old' "$ROOT/compose.yaml"
echo "PASS: immutable candidate replaced only target; sibling ID/image/labels/networks/mounts unchanged"
echo "NOTE: fixture uses disposable localhost registry, not production OCI trust or approval."
