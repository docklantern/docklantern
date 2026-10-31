package store

import (
    "context"
    "testing"
    "time"
)

// This integration scenario couples beta.5's discovery, persistent watch
// targets and Check Now scheduling. It does not use a Docker socket or
// external registry and is safe to run under go test -race.
func TestBeta5WatchOverrideManualCheckKeepsPlatformAndOptOutIsolation(t *testing.T) {
    ctx := context.Background()
    s := newLabelTestStore(t)
    amd := testLabelContainer("alpha", "web", "nginx:stable",
        map[string]string{labelMonitor: "true"})
    arm := testLabelContainer("arm", "web", "nginx:stable",
        map[string]string{labelMonitor: "true"})
    arm.ImageArchitecture = "arm64"
    disabled := testLabelContainer("blocked", "web", "nginx:latest",
        map[string]string{labelMonitor: "false"})
    doSnapshot(t, s, amd, arm, disabled)

    before, err := s.MonitorPolicies(ctx)
    if err != nil || len(before) != 2 {
        t.Fatalf("expected two enrolled policies: %+v %v", before, err)
    }
    now := time.Now().UTC().Truncate(time.Second)
    // Establish independent, recently successful baselines.
    for _, p := range before {
        if err := s.FinishMonitorCheck(ctx, p, now.Add(-2*time.Minute), nil); err != nil {
            t.Fatal(err)
        }
    }

    // Only the amd64 service changes its watched registry tag. Its Docker
    // image stays nginx:stable, and arm64 retains the original watch.
    target := "nginx:latest"
    if err := s.SetServiceWatchOverride(ctx, "alpha", "web", &target); err != nil {
        t.Fatal(err)
    }
    after, err := s.MonitorPolicies(ctx)
    if err != nil || len(after) != 2 {
        t.Fatalf("policies lost after watch edit: %+v %v", after, err)
    }
    watched := "registry-1.docker.io/library/nginx:latest"
    original := "registry-1.docker.io/library/nginx:stable"
    if after[0].Project != "alpha" || after[0].Image != watched ||
        after[0].Platform != "linux/amd64" || after[1].Project != "arm" ||
        after[1].Image != original || after[1].Platform != "linux/arm64" {
        t.Fatalf("cross-platform or cross-service watch corruption: %+v", after)
    }
    observed, err := s.Containers(ctx)
    if err != nil || len(observed) != 3 {
        t.Fatalf("discovery records lost: %+v %v", observed, err)
    }
    for _, c := range observed {
        if c.Project == "alpha" && c.Image != "nginx:stable" {
            t.Fatalf("watch edit modified Docker-observed image: %s", c.Image)
        }
    }

    // The changed target is due and must not be rescheduled. A request for
    // an unobserved platform cannot cause a registry check or create a policy.
    requestAt := now.Add(time.Second)
    state, err := s.RequestImageCheck(ctx, watched, "linux/amd64", requestAt)
    if err != nil || state.State != CheckAlreadyDue {
        t.Fatalf("changed target check: %+v %v", state, err)
    }
    state, err = s.RequestImageCheck(ctx, watched, "linux/arm64", requestAt)
    if err != nil || state.State != CheckNotEligible {
        t.Fatalf("unwatched image platform accepted: %+v %v", state, err)
    }
    state, err = s.RequestServiceCheck(ctx, "blocked", "web", requestAt)
    if err != nil || state.State != CheckNotEligible {
        t.Fatalf("disabled-by-label service accepted: %+v %v", state, err)
    }

    // The arm64 watch has not changed and its ordinary next due is in the
    // future; Check Now schedules only that platform, not the amd64 watch.
    state, err = s.RequestImageCheck(ctx, original, "linux/arm64", now.Add(75*time.Second))
    if err != nil || state.State != CheckScheduled || state.Scheduled != 1 {
        t.Fatalf("arm64 check not independently scheduled: %+v %v", state, err)
    }
    current, err := s.MonitorPolicies(ctx)
    if err != nil { t.Fatal(err) }
    if current[0].Generation != after[0].Generation ||
        current[1].Generation <= after[1].Generation {
        t.Fatalf("platform-scoped scheduling affected other watch: %+v", current)
    }

    // A stale result from the pre-override amd64 watch must not overwrite
    // the new target's last attempt or scheduling generation.
    if err := s.FinishMonitorCheck(ctx, before[0], requestAt, nil); err != nil {
        t.Fatal(err)
    }
    latest, err := s.MonitorPolicies(ctx)
    if err != nil { t.Fatal(err) }
    if latest[0].Image != watched || latest[0].LastCheckedAt != "" {
        t.Fatalf("stale pre-override result committed: %+v", latest[0])
    }

    // Reset is also a monitoring-only preference update.
    if err := s.SetServiceWatchOverride(ctx, "alpha", "web", nil); err != nil {
        t.Fatal(err)
    }
    restored, err := s.MonitorPolicies(ctx)
    if err != nil { t.Fatal(err) }
    if restored[0].Image != original || restored[0].Platform != "linux/amd64" ||
        restored[1].Image != original || restored[1].Platform != "linux/arm64" {
        t.Fatalf("reset merged platforms or lost policy: %+v", restored)
    }
}
