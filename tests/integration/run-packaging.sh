#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${DOCKLANTERN_INTEGRATION_TEST:-}" != "1" ]]; then
  echo "Use DOCKLANTERN_INTEGRATION_TEST=1 on disposable hosts only" >&2
  exit 2
fi
for cmd in docker curl mktemp; do command -v "$cmd" >/dev/null || exit 2; done
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
name="docklantern-package-$RANDOM-$RANDOM"
image="docklantern-ci:$RANDOM"
port=18787
cleanup(){
 docker rm -f "$name" >/dev/null 2>&1 || true
 docker image rm "$image" >/dev/null 2>&1 || true
 rm -rf "$tmp"
}
trap cleanup EXIT
mkdir -p "$tmp/data" "$tmp/secrets"
printf 'a-unique-long-test-password-123456789\n' > "$tmp/secrets/admin_password"
chmod 600 "$tmp/secrets/admin_password"

docker build --build-arg VERSION=v0.0.0-ci --tag "$image" --file "$repo/Dockerfile" "$repo"
[[ "$(docker run --rm "$image" -version)" == "DockLantern v0.0.0-ci" ]] || { echo "release version missing from image" >&2; exit 1; }
# Regression: the publicly exposed monitoring service never needs the powerful
# docker, docker compose or docker buildx clients (including their dependencies).
docker run --rm --entrypoint /bin/sh "$image" -ec '
  if command -v docker >/dev/null 2>&1;then
    echo "Docker CLI unexpectedly packaged in monitoring runtime" >&2
    exit 1
  fi
'
docker run -d --name "$name" --publish "127.0.0.1:$port:8787" \
 --mount "type=bind,src=$tmp/data,dst=/data" \
 --mount "type=bind,src=$tmp/secrets/admin_password,dst=/run/secrets/admin_password,readonly" \
 -e DOCKLANTERN_ADMIN_USERNAME=admin \
 -e DOCKLANTERN_ADMIN_PASSWORD_FILE=/run/secrets/admin_password \
 -e DOCKLANTERN_ALLOW_PROXY=1 \
 -e DOCKLANTERN_PUBLIC_ORIGIN=https://docklantern.example.test \
 "$image"

for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null; then break; fi
  sleep 1
done
curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null
for _ in $(seq 1 20); do
 if curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1; then break; fi
 sleep 1
done
curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null

status="$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: docklantern.example.test' -H 'X-Forwarded-Proto: https' "http://127.0.0.1:$port/")"
[[ "$status" == 401 ]] || { echo "expected 401, got $status" >&2; exit 1; }
status="$(curl -s -o /dev/null -w '%{http_code}' -u 'admin:a-unique-long-test-password-123456789' -H 'Host: docklantern.example.test' -H 'X-Forwarded-Proto: https' "http://127.0.0.1:$port/")"
[[ "$status" == 200 ]] || { echo "expected 200, got $status" >&2; exit 1; }
status="$(curl -s -o /dev/null -w '%{http_code}' -u 'admin:a-unique-long-test-password-123456789' -H 'Host: other.example.test' -H 'X-Forwarded-Proto: https' "http://127.0.0.1:$port/")"
[[ "$status" == 403 ]] || { echo "Host spoof accepted: $status" >&2; exit 1; }
status="$(curl -s -o /dev/null -w '%{http_code}' -u 'admin:a-unique-long-test-password-123456789' -H 'Host: docklantern.example.test' "http://127.0.0.1:$port/")"
[[ "$status" == 403 ]] || { echo "plaintext proxy header accepted: $status" >&2; exit 1; }

# The deployment example must resolve in Compose without publishing a host port.
mkdir -p "$repo/deploy/traefik/secrets"
printf 'testing-only-not-a-real-admin-password-123\n' > "$repo/deploy/traefik/secrets/admin_password.txt"
trap 'rm -f "$repo/deploy/traefik/secrets/admin_password.txt"; cleanup' EXIT
DOCKLANTERN_DOMAIN=docklantern.example.test DOCKLANTERN_ADMIN_USERNAME=admin TRAEFIK_CERTRESOLVER=letsencrypt \
 docker compose -f "$repo/deploy/traefik/compose.yaml" config --format json > "$tmp/render.json"
if grep -q '"ports":' "$tmp/render.json";then echo "unexpected published port" >&2;exit 1;fi

echo "PASS: container image, auth, proxy origin, secret file, HTTPS Compose template"
