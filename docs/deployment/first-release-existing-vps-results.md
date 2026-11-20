# Existing VPS functional acceptance — first-release RC

**Recorded:** 2026-11-20 UTC. **Result:** functional PASS with documented
exceptions on a disposable existing VPS; **not** production or release
sign-off. Clean-VPS acceptance, remaining negative checks and separate release
approval are outstanding. Evidence is from operator-supplied VPS command
output and GUI confirmations, not a new independent host inspection.

## Tested configuration

- Source: `9be9334d7e6923c1a14a5d090a66c474d32c1d8f`.
  Exact-source [Go CI](https://github.com/docklantern/docklantern/actions/runs/43804403254)
  and [security audit](https://github.com/docklantern/docklantern/actions/runs/43804403314)
  completed successfully; this is not a new audit of the final release artifact.
- Monitor: `docklantern-rc:9be9334d7e69`, local image ID
  `sha256:3c4346f151e5947b05f559ac77827b972cef43dfe6b669bbb77e88f571243418`.
- Executor: `/usr/local/libexec/docklantern-executor`,
  version `v0.1.0-rc.9be9334d7e69`. Full verified bundle and binary
  checksums should be retained in the private operator acceptance evidence;
  they are not reproduced here.
- Ubuntu 24.04, Linux x86_64, IPv6-only VPS; Docker client/server 29.8.1,
  API 1.56, Compose 5.5.1. Exact compatibility gate retained.
- Root-protected registration `hello-go-rc`, disposable single service
  `web`, public Docker Hub repository `docker.io/davidslusser/hello-go`,
  watched/authorized tag `latest`, linux/amd64, persistent override and
  explicit running-only verification (no Docker healthcheck).
- Root-owned backup baseline established; off-host custody and independent
  restore drill not established by this record.
- One enabled timer instance `docklantern-auto@hello-go-rc.timer`; no
  monitoring-container Docker write access.

## Live deployment evidence

Both deployments were timer-triggered, not manually applied.

| Version | OCI index digest (sha256) | Container ID after deployment |
| --- | --- | --- |
| 1.0.5 baseline | `2784c9e97340a33997905ad651c034fe11dba9f3a1ab8a2218ccffc093c1a603` | `df82f6c05a72d399b35349b5032a8efe07d19b2c5dcea1d541f75438cabb964a` |
| 1.0.6 | `039776ca889662ac65c145ec16c25113f5a929449dec793e203bdafa5a736267` | `c27e68a1c963bf4ce71348801aaa5f598b5f6cdc6dfe769cd0d87f75e1ca80cd` |
| 1.0.7 | `30e1a46b1aec8510119b28b93a16f462a8f017cd4809fe144d86b7c44ed9ccad` | `04369b8bf09889f89a9994002f11bc6a696532944b5879c014d3410ec5519460` |

The 1.0.6 and 1.0.7 linux/amd64 platform manifests were respectively
`sha256:c6d69c1dfe535e75c3a7810c692c9d75b748260c507959618f3a59a7f1393370`
and `sha256:3ccab080d6f0b05f1b700b6547ffe0d55461230cf694a58e965a1f67c9040da2`.

| Operation | UTC start/end | Journal | Override generation |
| --- | --- | --- | --- |
| `apply-a317695cafdf8bb59775a161` | 04:54:51–04:55:09 | COMMIT / succeeded | 1 |
| `apply-41073e5cdd4b5d1229213291` | 05:15:40–05:15:55 | COMMIT / succeeded | 2 |

Independent Docker inspection confirmed the expected immutable image and
running target; the second image and container embedded `IMAGE_TAG=1.0.7`.
The target had no healthcheck, so this is running-only evidence, not an
application-health or data-migration guarantee. Both unchanged-image checks
preserved the target container. All seven unrelated containers retained
their identities and running state across both deployments.

The second image's versioned and latest Docker Hub tags matched the new index.
The approved test publication used commit
`1648a4d36e3ce9590e829aa2f6eaa390286b31bf` in the hello-go repository.
The resulting digest was verified after publication, rather than before
the tag push, because the normal workflow builds and promotes latest
together. GHCR publication was not independently verified in the second test.

## Persistence and authority isolation

Default-file Compose recreation and restart (timer paused, executor idle)
preserved the 1.0.7 digest. Recreation yielded container
`212e31ffa47ad448cbb42f5377de07b738f79e325708a2cf55991ac2db6f9e2d`;
restart retained it. Registration, base Compose source, override and receipt
hashes were unchanged; the protected journal operation and generation
snapshots were unchanged. At 05:36:35–05:36:37 UTC the resumed timer reported
unchanged image, exit 0, and preserved the recreated target.

The second receipt was published with pending and published journal events,
`COMMIT / succeeded`, message code `verified_update`, and exact content
SHA-256 `f8be70ef616dab2cee51e659941a13d1cccb813de19c831bd5369ac29912ad93`.
The receipt and its directory were root-owned, modes 0644 and 0755.

Only the monitor was subsequently recreated with the added read-only receipt
bind. Existing image ID, persistent volume, mounts, environment and networks
were preserved; all seven other containers remained unchanged and running.
New monitor ID:
`2ea84b722aca1505806bdf34d7f666541a36c9351933d2b312323cc3ebee9275`.
All nine final GUI checks passed, including positive Activity receipt
visibility. The independently scheduled executor ran concurrently with
monitor recreation at 05:46:43–05:46:46 UTC and reported unchanged image.
Both container identities were preserved at the final inspection.

Existing-host Compose order (host-local overlays, not a universal install):
`compose.yaml`, `compose.discovery.yaml`, `compose.beta6-release.yaml`,
`compose.rc-9be9334d7e69.yaml`, `compose.receipts.yaml`.
Preserve the entire ordered stack for future monitor recreation.

## Exceptions and release gates

- The first receipt was absent because the outbox was mode 0700. An approved
  correction to 0755 enabled second receipt publication. Do not fabricate or
  backfill the first receipt; a no-op tick does not republish it.
- The installed systemd service template was corrected from the unsupported
  `ConditionPathIsExecutable` to `ConditionFileIsExecutable`; local validation
  passed. [PR #138](https://github.com/docklantern/docklantern/pull/138)
  was subsequently merged at `560ee976e773ed84b285d81e9c1b8ff2b7389667`.
  The source delta from the tested commit is only that systemd template;
  the running artifacts have not been rebuilt or redeployed.
- Capture a strictly post-monitor-recreation scheduled check: the recorded
  last tick overlapped recreation, proving independence but not strictly
  later execution.
- Complete the applicable negative tests in the
  [RC runbook](first-release-rc-acceptance.md), including evidence of any
  existing isolated CI coverage and explicit outstanding cases. Do not infer
  these passed from the positive live tests.
- Complete clean-VPS acceptance with a fresh journal/registration, verified
  paired artifacts, correctly provisioned outbox before the timer, two new
  distinct approved images, positive receipts for both, no-op and persistence
  checks. Record exact artifact hashes and CI evidence, proxy restrictions,
  backups and restore evidence or explicit disposable-host-only waivers.
- Manual recovery, off-host restore, remaining adversarial cases and broader
  GUI proposal functionality retain the limitations tracked in #79, #85–#88.
  No production-readiness claim or release authorization follows from this
  disposable-host functional result.

No release, deployment, timer change or merge is authorized by this record.
