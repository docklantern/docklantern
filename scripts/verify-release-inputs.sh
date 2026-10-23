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
echo "PASS: Dockerfile builder and runtime match reviewed OCI digest lock"
