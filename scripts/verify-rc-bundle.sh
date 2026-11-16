#!/usr/bin/env bash
# Read-only checksum and provenance verification for a previously built RC
# directory. This is an integrity check, not a cryptographic signature.
set -Eeuo pipefail
if [[ "$#" -lt 1 || "$#" -gt 2 || "$1" != /* ]]; then
  echo "Usage: bash scripts/verify-rc-bundle.sh /absolute/bundle [expected_40_char_commit]" >&2
  exit 2
fi
dir="$1"
[[ -d "$dir" && ! -L "$dir" ]] || exit 2
for f in monitor-linux-amd64.tar docklantern-executor provenance.txt SHA256SUMS; do
  [[ -f "$dir/$f" && ! -L "$dir/$f" && -s "$dir/$f" ]] || exit 2
done
(
  cd "$dir"
  sha256sum --check --strict SHA256SUMS
)
commit="$(sed -n 's/^source_commit=//p' "$dir/provenance.txt")"
version="$(sed -n 's/^version=//p' "$dir/provenance.txt")"
builder="$(sed -n 's/^executor_builder=//p' "$dir/provenance.txt")"
[[ "$commit" =~ ^[a-f0-9]{40}$ &&
   "$version" == "v0.1.0-rc.${commit:0:12}" &&
   "$builder" == golang:*@sha256:* ]] || exit 2
grep -Fxq 'platform=linux/amd64' "$dir/provenance.txt"
grep -Fxq 'published=false' "$dir/provenance.txt"
[[ "$#" -eq 1 || "$commit" == "$2" ]] || {
  echo "RC source commit does not match the reviewed commit" >&2
  exit 2
}
echo "PASS: bundled checksums and source provenance verified for $commit"
echo "WARNING: this verifies local integrity, not publisher identity or live Docker state."
