# Beta.3: Opt-in Compose image monitoring

**Status:** Proposed feature on [PR #38](https://github.com/docklantern/docklantern/pull/38), **not available in beta.2**. The GUI interval editor is planned separately in issue #30.

## Enroll a new service without editing DockLantern

After DockLantern is installed with periodic Docker inventory scanning and the restricted read-only Docker socket proxy enabled, add a label to the **application's own** Compose service:

```yaml
services:
  web:
    image: example/hello-js:latest
    labels:
      com.docklantern.monitor: "true"
      # Optional: defaults to the global interval (initially 5 minutes)
      com.docklantern.interval: "10m"
```

Recreate **only that application's service** using your normal Compose deployment flow to apply the labels. The next DockLantern discovery scan reads the new container labels and creates a monitoring policy in SQLite. **DockLantern itself does not need any Compose edits, `-roots`, original Compose source mounts, project registration, or restart for monitoring.**

No label means an otherwise unregistered service is displayed in inventory but is not monitored by default. To explicitly disable monitoring:

```yaml
labels:
  com.docklantern.monitor: "false"
```

The `false` setting overrides legacy registration and later GUI enrollment. Removing `true` disables label-owned checks unless the administrator independently enrolled the service in the GUI.

## Supported labels

| Label | Accepted values | Default |
| --- | --- | --- |
| `com.docklantern.monitor` | `"true"`, `"false"` or absent | Absent = not automatically enrolled |
| `com.docklantern.interval` | Whole-second Go durations from `60s` through `24h`, e.g. `5m`, `10m`, `1h` | Global 300 seconds (5m) |

When both a GUI interval override and interval label exist, the GUI override wins. With no override, the interval label wins; otherwise the global default applies. **The GUI controls and live rescheduling are part of issue #30**, not PR #38.

A service must have a tag-based registry image reference (e.g. `nginx:1.27`) and verifiable image OS/architecture from Docker Engine image inspection. Missing metadata, invalid labels or conflicting replicas make the service **inactive**, with a validation reason recorded for later display. A Docker proxy connection failure does not erase existing enrollment.

Replicated services are monitored once per Compose project/service. DockLantern does not follow new Docker events instantly; it observes labels on its configured periodic discovery scan.

## Existing beta.2 services

Registered monitoring policies from beta.2 retain their image, scheduling, pause/backoff state and registry history. DockLantern **does not overwrite** these with automatically discovered label values. Operators can keep them running without adding any labels, although `monitor=false` explicitly suspends all checks for that service.

The source-root mounts used for beta.2 project registration are still needed if continuing to use the legacy registration form; **they are not required for label-based monitoring**. They also remain relevant to future source-validated Compose deployment operations, which this beta does **not** implement.

## Safety

Image monitoring makes read-only Docker GET and OCI manifest requests. It never pulls, restarts or recreates a container, and a label does not grant permission to run Compose deployments. Keep the Docker socket proxy private, GET-only and restricted to necessary endpoints.

Before upgrading a database, follow the [beta.3 SQLite backup and restore guide](../deployment/beta3-sqlite-upgrade.md). The beta.3 schema migration is not guaranteed to be readable by an older beta binary.
