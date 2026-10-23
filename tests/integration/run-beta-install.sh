#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "${DOCKLANTERN_INTEGRATION_TEST:-}" != "1" ]];then
  echo "Beta installation smoke tests require an explicitly disposable Docker host" >&2
  exit 2
fi
for command in docker mktemp;do command -v "$command" >/dev/null || exit 2;done
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
compose="$root/tests/integration/beta-install/compose.yaml"
scratch="$(mktemp -d)"
export COMPOSE_PROJECT_NAME="docklantern-beta-$RANDOM-$RANDOM"
export DOCKLANTERN_BETA_TEST_IMAGE="docklantern-beta-ci:$RANDOM"
export DOCKLANTERN_BETA_TEST_FIXTURE_DIR="$scratch/fixtures"
export DOCKLANTERN_BETA_TEST_PASSWORD_FILE="$scratch/admin_password"
cleanup(){
  status=$?
  if ((status != 0));then
    echo "=== Disposable DockLantern container diagnostics ===" >&2
    docker compose -f "$compose" ps -a >&2 || true
    docker compose -f "$compose" logs --no-color --tail 100 >&2 || true
  fi
  docker compose -f "$compose" down -v --remove-orphans >/dev/null 2>&1 || true
  docker image rm "$DOCKLANTERN_BETA_TEST_IMAGE" >/dev/null 2>&1 || true
  rm -rf "$scratch"
}
trap cleanup EXIT
mkdir -p "$DOCKLANTERN_BETA_TEST_FIXTURE_DIR/project"
printf 'beta-disposable-test-strong-secret-123456789\n' > "$DOCKLANTERN_BETA_TEST_PASSWORD_FILE"
chmod 600 "$DOCKLANTERN_BETA_TEST_PASSWORD_FILE"
cat > "$DOCKLANTERN_BETA_TEST_FIXTURE_DIR/project/compose.yaml" <<'YAML'
services:
  web:
    image: nginx:alpine
YAML
cat > "$DOCKLANTERN_BETA_TEST_FIXTURE_DIR/registration.json" <<'JSON'
{"name":"smoke","project_dir":"/fixtures/project","files":["/fixtures/project/compose.yaml"],"services":["web"]}
JSON

docker build -q --build-arg VERSION=v0.1.0-beta.1-ci -t "$DOCKLANTERN_BETA_TEST_IMAGE" "$root"
[[ "$(docker run --rm "$DOCKLANTERN_BETA_TEST_IMAGE" -version)" == "DockLantern v0.1.0-beta.1-ci" ]]
# No host-published ports. All sensitive data remains in a disposable volume.
docker compose -f "$compose" config --format json > "$scratch/resolved.json"
if grep -q '"ports":' "$scratch/resolved.json";then echo "unexpected host port exposure" >&2;exit 1;fi
docker compose -f "$compose" up -d --no-build
for i in $(seq 1 30);do
 if docker compose -f "$compose" exec -T docklantern wget -q -O - http://127.0.0.1:8787/readyz 2>/dev/null | grep -q '"status":"ready"';then break;fi
 sleep 1
done
docker compose -f "$compose" exec -T docklantern wget -q -O - http://127.0.0.1:8787/readyz | grep -q '"status":"ready"'
# Register a monitoring-only service using original Compose config files.
docker compose -f "$compose" exec -T docklantern docklantern-inventory -mode register -db /data/docklantern.db -roots /fixtures/project -registration /fixtures/registration.json
docker compose -f "$compose" exec -T docklantern docklantern-inventory -mode projects -db /data/docklantern.db > "$scratch/projects-before.json"
grep -q '"name":"smoke"' "$scratch/projects-before.json"
# Create an online consistent snapshot without stopping the server.
docker compose -f "$compose" exec -T docklantern docklantern-inventory -db /data/docklantern.db -backup /data/snapshot.sqlite
# Verify persistence across restart.
docker compose -f "$compose" restart docklantern
docker compose -f "$compose" exec -T docklantern docklantern-inventory -mode projects -db /data/docklantern.db > "$scratch/projects-after.json"
grep -q '"name":"smoke"' "$scratch/projects-after.json"
# Restore into a stopped, isolated database; remove old WAL and SHM alongside old DB.
docker compose -f "$compose" stop docklantern
docker compose -f "$compose" run --rm --no-deps --entrypoint sh docklantern -ec '
  mv /data/docklantern.db /data/docklantern.db.before-restore
  if [ -f /data/docklantern.db-wal ]; then mv /data/docklantern.db-wal /data/docklantern.db-wal.before-restore; fi
  if [ -f /data/docklantern.db-shm ]; then mv /data/docklantern.db-shm /data/docklantern.db-shm.before-restore; fi
  cp /data/snapshot.sqlite /data/docklantern.db
  chmod 600 /data/docklantern.db
'
docker compose -f "$compose" start docklantern
docker compose -f "$compose" exec -T docklantern docklantern-inventory -mode projects -db /data/docklantern.db > "$scratch/projects-restored.json"
grep -q '"name":"smoke"' "$scratch/projects-restored.json"
echo "PASS: isolated Compose installation, readiness, monitoring enrollment, online backup, restart and restore"
