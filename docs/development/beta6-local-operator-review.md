# Beta.6 Phase 2c — Protected registration publication and review-only TTY gate

**Status:** Proposed implementation for review under [B6-02 (#82)](https://github.com/docklantern/docklantern/issues/82). **This is not an operational deployment authorization** and does not qualify any Docker/Compose version for writes.

## Scope delivered by this review slice

The host-owned registration publisher in `internal/executorregistration` is designed to create or replace a protected registration record. Its **only exported production entry**, `PublishProtected`, requires an actual **root EUID** and a controlling **`/dev/tty`**, rechecks original source owners after the operator types the precise registration ID and short SHA-256 confirmation challenge, and never calls Docker or writes Compose originals.

- The protected registration directory and all ancestors must already exist with trusted root ownership. No directory creation or permission migration is performed.
- Publication uses `openat(O_NOFOLLOW)` for trusted directory components, `flock` for concurrent cooperating publishers, random `O_EXCL|O_NOFOLLOW` temporary files (`0600`), `fsync`, and atomic `renameat` with a directory `fsync`.
- Creation requires that no prior record exists. Replacement requires the **exact SHA-256 of the existing protected record** supplied by the authorized host operator; mismatches fail closed. Existing protected registration and trusted source validation is repeated before replacement.
- No control is exposed in the web UI, no registration path may come from an untrusted proposal, and the existing CLI `register` command remains **unavailable**. The core publisher is **not installed or invoked on any real host** in this PR.
- The automated root fixture calls the unexported publisher only on an isolated disposable directory, testing create, update, stale hash rejection, unsafe ownership and modes. No real `/var/lib/docklantern` state is touched.

The new `internal/executorreview` library supports a **review-only**, host-local operator acknowledgment with no bearer capability:

1. An independent reader loads a proposed plan and its exact-byte fingerprint from the fixed, monitor-writable inbox using a separately trusted **nonroot monitor UID**.
2. Independently loads the protected root-owned registration and requires exact registration ID, enabled service, repository and platform eligibility.
3. Requires a **separate trusted inspector** to read an actual Docker/container baseline and stable Compose source fingerprint, with maximum 10-second freshness. **No real production inspector has been implemented or connected**; Phase 4 owns those requirements.
4. Requires a real root EUID and controlling `/dev/tty` for the production terminal adapter; no stdin, web or `--yes` fallback. Prints exact project, service, repository, immutable candidate digest, platform, expiry, registration/proposal content fingerprints, actual baseline fingerprint, and weaker `allow_running_only` readiness disclosure when relevant.
5. Generates a new cryptographically random 12-hex confirmation challenge for each review, binding the exact plan/registration/baseline hashes and candidate. The host operator must type `REVIEW <challenge>` on the TTY.
6. **After** local input, rereads and revalidates the registration, proposal and freshly inspected baseline, compares all three fingerprints and checks the deadline again. Changed, stale, expired, non-TTY, nonroot or unauthorized inputs fail closed with generic errors.
7. Emits only a `ReviewOnly` informational receipt. **It cannot authorize `apply`, `recover` or any Docker write.** Approval is not reusable or passed to any execution layer.

**Follow-up integration:** The separately reviewed Phase 2 register-CLI slice enables only root/TTY host registration publication. See [host registration CLI](beta6-register-cli.md). `preview`, `apply`, `recover` and `reconcile` remain disabled. The Phase 2c implementation described below originally shipped only internal library functions.

## Explicit restrictions and deferred gates

At the original Phase 2c merge, all five commands were disabled; the follow-up [register-CLI integration](beta6-register-cli.md) enables only trusted host-local registration publication. `preview`, `apply`, `recover` and `reconcile` remain disabled, and protected operator/monitor-UID configuration for preview is a separate future gate.

There is no qualified production `InspectActual` implementation: passing a fake baseline in CI does not establish trusted Docker or Compose state. Full interactive execution approval requires **B6-04 stable source snapshots, exact supported Engine/Compose compatibility, immutable OCI evidence and image-only changes**, plus B6-03 durable journal/locks. Only then may a subsequent separately approved Phase 5 introduce Docker mutations.

The older `Lstat` source trust preflight remains **not TOCTOU-proof**. No test-VPS changes, UID/mount provisioning, write-capable Docker integration, releases, host registration files or backups have been performed or authorized. The live `hello-go` ancestor at mode `0775` remains ineligible.

## Tests and remaining acceptance

- Fake-driven unit tests cover nonroot/non-TTY rejection, missing/stale Docker baseline, unauthorized service/repository, exact preview disclosure, plan/registration/baseline mutation after entry, expiration while typing and challenge replay rejection. No Docker commands are executed by these tests.
- Existing disposable root-owned registration fixture is extended to test atomic create and compare-and-replace, no silent overwrite, bad expected hash, unsafe project permissions and clean temporary-file disposal.
- Go race tests, vet, existing proxy/integration regressions and actual executor binary security scanning remain CI gates. A root-owned CI fixture is **not evidence** of real VPS UID or mount isolation; an actual disposable host/PTY exercise must be approved before shipping this for host use.

**Governance:** PR open for review only. Do not merge, publish, install, change VPS permissions or deploy Docker workloads without the user's explicit separate permission. Keep Phase 2 issue #82 open until the remaining host-local CLI integration and full acceptance are evidenced.

## Read-only local preview follow-up

The separate [candidate preview integration](beta6-local-preview.md) provides an informational root/TTY CLI display of matched protected registration plus untrusted proposal. It **does not** invoke the approval gate, does not inspect Docker actual state, and does not grant a deployment capability. B6-04 actual-state/Compose/OCI inspection remains a hard gate; `apply` and `recover` remain disabled.
