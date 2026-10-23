# PR #14 — Guided enrollment and evidence-based image status

## Goal

Assist users in configuring monitoring for discovered Compose services, and **avoid false claims that an image update is available**.

## Guided enrollment

The authenticated `/settings` page displays discovered containers and provides a **Prefill** action. Selecting a row copies a previously discovered project's name, service, and image reference into the existing registration and monitoring forms. The user must still provide the original Compose file paths and explicitly register the service. Server-side selection requires a matching record from the stored Docker inventory; the query string cannot invent a Compose project. Registration still validates allowlisted host source paths and keeps `enabled=false`.

No container is ever enrolled automatically. No Docker commands are issued from these actions.

## Image status semantics

A new read-only endpoint `GET /api/v1/image-assessments` and a dashboard section combine explicitly registered services, stored container snapshots, monitoring policies, and the **last successfully checked** remote OCI digest.

Possible results:

| State | Meaning |
|---|---|
| `not_checked` | No successful remote registry observation for the watched image/platform |
| `not_running` | No recorded containers for that registered service |
| `paused` | Monitoring policy paused |
| `unknown` | The observed container image reference isn't provably pinned to a manifest digest, or replicas disagree |
| `current` | Every recorded replica has an immutable, matching repository/digest reference equal to the remote index or platform digest |
| `different` | Immutable recorded digest differs from the remote watched tag's most recent digest |
| `unregistered` | Policy's project/service is not found in current registered allowlist |

**Different does not mean newer**. It is possible for a remote tag to have moved backwards, and the current snapshot/remote check may be stale. It means only that the digests differ. The dashboard deliberately does not display an "Update available" banner.

**Docker image configuration IDs are not OCI manifest digests.** The existing Docker scan records container `Image` (local config ID) and `Config.Image` (original launch reference). The assessment uses only the latter when it contains a valid `@sha256:<64 lowercase hex>` pinned reference for the *same normalized repository* as the watched policy. Ordinary tagged deployments must remain unknown until DockLantern can query trusted image manifest provenance and correlate it with running containers. Multiple replicas require identical pinned references.

An image assessment is only as fresh as the inventory snapshot and registry check; time-based freshness requirements and platform resolution for multi-arch tags require further implementation.

## Security

No additional Docker socket endpoints are exposed, no image pulls, no deployment controls and no write APIs are added. The dashboard and JSON endpoint remain authenticated by the existing guard. Prefill values come from stored observations, render through Go's `html/template`, and still require the normal CSRF-protected registration and policy forms.

Future work: verified `RepoDigests` provenance via a restricted and audited Docker image inspect capability, exact platform correlation, freshness thresholds, and safe release/version classification.
