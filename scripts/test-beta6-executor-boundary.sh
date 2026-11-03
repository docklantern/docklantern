#!/usr/bin/env bash
set -Eeuo pipefail

# CI-only disposable Unix permission verification. Does not access Docker.
if [[ "$(printenv CI || true)" != "true" && "$(printenv DOCKLANTERN_BOUNDARY_TEST || true)" != "1" ]]; then
  echo 'Run only on a disposable CI host (CI=true) or explicitly approved fixture' >&2
  exit 2
fi
if [[ "$(uname -s)" != Linux ]]; then
  echo 'Linux is required' >&2
  exit 2
fi
command -v sudo >/dev/null || { echo 'sudo required for fixture ownership' >&2; exit 2; }
id nobody >/dev/null || { echo 'nobody user required for nonroot attacker fixture' >&2; exit 2; }

root="$(mktemp -d)"
trap 'sudo rm -rf -- "$root"' EXIT
# Only this unique root is touched. Mode 0711 allows the untrusted user to
# traverse to its proposal leaf, not list trusted parents.
sudo chown root:root "$root"
sudo chmod 0711 "$root"
sudo mkdir -p "$root/handoff/proposals" "$root/handoff/receipts" "$root/executor/journal"
sudo chown root:root "$root/handoff" "$root/handoff/receipts" "$root/executor" "$root/executor/journal"
sudo chmod 0711 "$root/handoff"
sudo chmod 0700 "$root/executor" "$root/executor/journal"
sudo chmod 0755 "$root/handoff/receipts"
sudo chown nobody "$root/handoff/proposals"
sudo chmod 0700 "$root/handoff/proposals"

sudo -u nobody touch "$root/handoff/proposals/submitted.json"
sudo -u nobody test -f "$root/handoff/proposals/submitted.json" || { echo "monitor could not read its own proposal" >&2; exit 1; }
if sudo -u nobody touch "$root/handoff/receipts/forged.json" 2>/dev/null; then
  echo 'monitor identity unexpectedly modified receipts' >&2; exit 1
fi
if sudo -u nobody touch "$root/executor/journal/forged.sqlite" 2>/dev/null; then
  echo 'monitor identity unexpectedly modified executor journal' >&2; exit 1
fi
# This checks Unix permissions, NOT actual monitoring container bind mounts.
echo 'PASS: nonroot monitor can write proposals but not receipts/journal'
