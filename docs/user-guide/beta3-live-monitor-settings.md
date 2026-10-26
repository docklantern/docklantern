# Beta.3: Live monitoring settings

**Status:** Implemented in beta.3 development PR; available after the beta.3 application release, not in the currently deployed beta.2 image.

Open **Settings → Monitoring defaults** to change the default polling interval (initially **300 seconds / 5 minutes**). Valid intervals range from **60 seconds to 24 hours**. Saving takes effect in SQLite immediately. Inheriting services receive the new interval and are promptly rescheduled; any ongoing registry rate-limit backoff remains in force.

## Per-service controls

After Docker inventory has successfully discovered a Compose service, **Settings → Service monitoring** shows its project/service identity, enrollment source, effective interval and configuration source, and validation/paused status.

- **GUI enrollment:** For a discovered running service without a monitoring opt-in label, select **Enable in GUI**. DockLantern verifies persisted Docker inventory metadata, then creates a monitoring policy without requiring Compose source mounts, `-roots`, or project registration. **Disable GUI enrollment** stops future checks without deleting history.
- **Optional Docker labels:** `com.docklantern.monitor: "true"` enables monitoring declaratively; `com.docklantern.interval: "10m"` sets the optional interval. You need to redeploy **the monitored Compose service** to apply changed labels, but not DockLantern itself.
- **GUI interval override:** Set a custom per-service interval (60–86400 seconds) and click **Save override**. It takes precedence over any Docker interval label. **Reset override** returns a label-managed service to its Docker label or the global default. Legacy beta.2 manually registered services reset to their preserved manual schedule.
- **Pause / Resume:** Pause prevents future scheduled checks; resume makes the service eligible for a prompt check without restarting any containers.
- **Monitoring status:** An explicit `com.docklantern.monitor=false` label overrides GUI enrollment; missing/stopped or invalid-discovery services are not automatically eligible. An interval override alone never enrolls a service.

## Precedence and provenance

For a new label/GUI-enrolled service, **GUI interval > Docker interval label > global default**. Legacy manually registered policies retain their original settings, with the possibility of temporary GUI overrides and reset to the original manual interval. Manual enrollment, label enrollment, and GUI enrollment remain distinct records.

The settings and overrides persist in SQLite across DockLantern restarts and Compose service recreations. All GUI form submissions require authenticated access plus CSRF and origin validation; they are not exposed as Docker commands.

A running/in-flight registry check may already be underway when you change settings. The storage uses a policy-generation guard to prevent its completion from overwriting updated schedules. An in-flight network request cannot be retroactively canceled through these forms.

## Security and upgrade

DockLantern remains **monitoring-only**: the interface does not pull images, restart containers, execute Compose, or grant deployment access. The discovery proxy remains limited to read-only Docker endpoints.

Before upgrading an existing beta.2 SQLite database, follow the [backup and recovery guide](../deployment/beta3-sqlite-upgrade.md). Versioned migrations preserve your existing monitoring history and registrations, but reversing a schema upgrade requires restoring a pre-upgrade database backup rather than assuming an old binary can read the new schema.
