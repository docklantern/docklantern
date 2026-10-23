#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
mapfile -t bases < <(grep -vE '^[[:space:]]*(#|$)' build/base-images.lock)
if [[ "${#bases[@]}" -ne 2 ]];then echo "Base image lock must contain exactly two entries" >&2;exit 1;fi
for base in "${bases[@]}";do
  [[ "$base" =~ ^[a-z0-9][a-z0-9./_-]*:[a-zA-Z0-9_.-]+@sha256:[a-f0-9]{64}$ ]] ||
    { echo "Invalid immutable base reference in lock" >&2;exit 1; }
done
grep -Fxq "FROM ${bases[0]} AS build" Dockerfile ||
  { echo "Build base does not match immutable lock" >&2;exit 1; }
grep -Fxq "FROM ${bases[1]}" Dockerfile ||
  { echo "Runtime base does not match immutable lock" >&2;exit 1; }
[[ "$(grep -c '^FROM ' Dockerfile)" == 2 ]] ||
  { echo "Unexpected additional unpinned Dockerfile stage" >&2;exit 1; }
proxy_base="$(cat build/proxy-base.lock)"
[[ "$proxy_base" =~ ^tecnativa/docker-socket-proxy:v[0-9]+\.[0-9]+\.[0-9]+@sha256:[a-f0-9]{64}$ ]] ||
  { echo "Optional proxy base must be immutable" >&2;exit 1; }
grep -Fxq "FROM $proxy_base" deploy/docker-socket-proxy/Dockerfile ||
  { echo "Optional privileged proxy base does not match lock" >&2;exit 1; }
echo "PASS: monitoring and privileged socket-proxy bases match immutable locks"
