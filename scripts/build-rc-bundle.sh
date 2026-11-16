#!/usr/bin/env bash
# Build two locally inspectable release-candidate artifacts from ONE clean
# source commit. No registry push, release tag, VPS connection or installation.
set -Eeuo pipefail

if [[ "${DOCKLANTERN_RC_BUILD:-}" != "1" || "$#" != 1 || "$1" != /* ]]; then
  echo "Usage: DOCKLANTERN_RC_BUILD=1 scripts/build-rc-bundle.sh /absolute/new/output-directory" >&2
  exit 2
fi
for cmd in git docker sha256sum; do command -v "$cmd" >/dev/null || exit 2; done
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo"
[[ -z "$(git status --porcelain --untracked-files=normal)" ]] || {
  echo "Refusing to label a dirty source tree with a reviewed commit" >&2; exit 2;
}
commit="$(git rev-parse --verify HEAD)"
[[ "$commit" =~ ^[a-f0-9]{40}$ ]] || exit 2
short="${commit:0:12}"
version="v0.1.0-rc.${short}"
image="docklantern-rc:${short}"
output="$1"
[[ "$output" != "/" && ! -e "$output" ]] || {
  echo "Refusing to overwrite an existing path" >&2; exit 2;
}
bash scripts/verify-release-inputs.sh
builder="$(sed -n 's/^FROM \\(golang:[^ ]*\\) AS build$/\\1/p' Dockerfile)"
[[ "$builder" == golang:*@sha256:* ]] || exit 2

# This local build DOES use Docker on the calling machine. Never point it
# at the VPS Docker daemon; CI runs it on disposable GitHub runners only.
[[ "$(docker context show)" == "default" ]] || {
  echo "Use only the local default Docker context for RC packaging" >&2; exit 2;
}
[[ -z "${DOCKER_HOST:-}" && -z "${DOCKER_CONTEXT:-}" ]] || {
  echo "Inherited Docker endpoint overrides are forbidden" >&2; exit 2;
}
mkdir -m 0700 "$output"
chmod 0700 "$output"
echo "Building local monitoring image ${image} from commit ${commit}"
docker build --platform linux/amd64 --build-arg "VERSION=$version" \
  --file Dockerfile --tag "$image" "$repo"
docker run --rm --platform linux/amd64 "$image" -version | grep -Fx "DockLantern $version"
docker run --rm --platform linux/amd64 --entrypoint /bin/sh "$image" -ec '
  ! command -v docker
  ! command -v docklantern-executor
'
docker save --output "$output/monitor-linux-amd64.tar" "$image"

# Build the separate host executor in the EXACT digest-pinned Go build image.
# Avoid using the workstation Go installation or inheriting local module cache.
docker run --rm --platform linux/amd64 \
  --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$repo,target=/src,readonly" \
  --mount "type=bind,source=$output,target=/out" \
  --workdir /src --env GOCACHE=/tmp/docklantern-gocache \
  --env GOPATH=/tmp/docklantern-gopath "$builder" sh -ec '
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
      -trimpath -buildvcs=false \
      -ldflags="-s -w -X github.com/docklantern/docklantern/internal/buildinfo.Version='"$version"'" \
      -o /out/docklantern-executor ./cmd/docklantern-executor
  '
chmod 0700 "$output/docklantern-executor"
docker run --rm --platform linux/amd64 \
  --mount "type=bind,source=$output,target=/out,readonly" \
  "$builder" /out/docklantern-executor version | grep -Fx "DockLantern Executor $version"

image_id="$(docker image inspect "$image" --format '{{.Id}}')"
[[ "$image_id" =~ ^sha256:[a-f0-9]{64}$ ]] || exit 2
{
  printf 'source_commit=%s\n' "$commit"
  printf 'version=%s\n' "$version"
  printf 'platform=linux/amd64\n'
  printf 'image_tag_local_only=%s\n' "$image"
  printf 'image_id=%s\n' "$image_id"
  printf 'executor_builder=%s\n' "$builder"
  printf 'published=false\n'
} > "$output/provenance.txt"
(
  cd "$output"
  sha256sum monitor-linux-amd64.tar docklantern-executor provenance.txt > SHA256SUMS
  sha256sum --check --status SHA256SUMS
)
printf 'PASS: paired local-only RC artifacts at %s\n' "$output"
printf 'No registry push, release tag, daemon installation or VPS change performed.\n'
