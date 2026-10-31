package store

import (
    "context"
    "path/filepath"
    "strings"
    "testing"
    "time"
)

const observedNginx = "registry-1.docker.io/library/nginx:latest"
const watchedStable = "registry-1.docker.io/library/nginx:stable"

// TestWatchTargetOverrideResetAndStaleCompletion proves an operator's watch
// choice never rewrites the running image, and stale scheduler work cannot
// overwrite the policy after its generation changes.
func TestWatchTargetOverrideResetAndStaleCompletion(t *testing.T) {
    ctx := context.Background()
    s := newLabelTestStore(t)
    c := testLabelContainer("app", "web", "nginx:latest", map[string]string{
        labelMonitor: "true", labelInterval: "10m",
    })
    doSnapshot(t, s, c)
    before := getOnePolicy(t, s)
    if before.Image != observedNginx { t.Fatalf("unexpected baseline %+v", before) }
    target := "nginx:stable"
    if err := s.SetServiceWatchOverride(ctx, "app", "web", &target); err != nil { t.Fatal(err) }
    after := getOnePolicy(t, s)
    if after.Image != watchedStable || after.Platform != "linux/amd64" ||
        after.IntervalSeconds != 600 || after.Paused ||
        after.LastCheckedAt != "" || after.ConsecutiveFailures != 0 ||
        after.Generation <= before.Generation {
        t.Fatalf("watch override modified unrelated policy state %+v", after)
    }
    if err := s.FinishMonitorCheck(ctx, before, time.Now().UTC(), nil); err != nil { t.Fatal(err) }
    if p := getOnePolicy(t, s); p.LastCheckedAt != "" {
        t.Fatalf("obsolete check completed after override %+v", p)
    }
    discovered, err := s.Containers(ctx)
    if err != nil || len(discovered) != 1 || discovered[0].Image != "nginx:latest" {
        t.Fatalf("Docker-observed image changed %+v %v", discovered, err)
    }
    doSnapshot(t, s, c)
    if p := getOnePolicy(t, s); p.Image != watchedStable {
        t.Fatalf("discovery overwrote explicit watch %+v", p)
    }
    config, err := s.MonitoringServiceSettings(ctx)
    if err != nil || len(config) != 1 || config[0].WatchImageOverride == nil ||
        *config[0].WatchImageOverride != watchedStable {
        t.Fatalf("preference not persisted %+v %v", config, err)
    }
    views, err := s.MonitorServiceViews(ctx)
    if err != nil || len(views) != 1 ||
        views[0].WatchSource != "GUI watch override" ||
        views[0].Image != watchedStable {
        t.Fatalf("watch provenance not exposed %+v %v", views, err)
    }

    // A paused service may edit its preference but must remain paused.
    if err := s.SetServicePaused(ctx, "app", "web", true); err != nil { t.Fatal(err) }
    if err := s.SetServiceWatchOverride(ctx, "app", "web", nil); err != nil { t.Fatal(err) }
    restored := getOnePolicy(t, s)
    if restored.Image != observedNginx || !restored.Paused ||
        restored.IntervalSeconds != 600 || restored.Generation <= after.Generation {
        t.Fatalf("reset failed or resumed service %+v", restored)
    }
    if len(dueAt(t, s, time.Now().Add(time.Minute))) != 0 {
        t.Fatal("paused watch became due after reset")
    }
    config, _ = s.MonitoringServiceSettings(ctx)
    if config[0].WatchImageOverride != nil { t.Fatal("reset did not remove override") }
    if err := s.SetServicePaused(ctx, "app", "web", false); err != nil { t.Fatal(err) }
    if len(dueAt(t, s, time.Now().Add(time.Minute))) != 1 { t.Fatal("resume not due") }
}

func TestWatchOverrideSameRepositoryDriftAndIdempotence(t *testing.T) {
    ctx := context.Background()
    s := newLabelTestStore(t)
    c := testLabelContainer("app", "web", "nginx:1.2.3", map[string]string{labelMonitor:"true"})
    doSnapshot(t, s, c)
    target := "nginx:latest"
    if err := s.SetServiceWatchOverride(ctx, "app", "web", &target); err != nil { t.Fatal(err) }
    p := getOnePolicy(t, s)
    if p.Image != observedNginx { t.Fatalf("wrong target %+v", p) }
    // Equivalent canonical reference must be an idempotent no-op.
    target = observedNginx
    if err := s.SetServiceWatchOverride(ctx, "app", "web", &target); err != nil { t.Fatal(err) }
    same := getOnePolicy(t, s)
    if same.Generation != p.Generation || same.NextCheckAt != p.NextCheckAt {
        t.Fatal("same tag needlessly rescheduled checks")
    }
    // An unrelated Compose rollout from 1.2.3 to 1.2.4 should not
    // replace the separate GUI watch and should not mutate it either.
    c.Image = "nginx:1.2.4"
    doSnapshot(t, s, c)
    if changed := getOnePolicy(t, s); changed.Image != observedNginx ||
        changed.Generation != same.Generation {
        t.Fatalf("same-repo Compose change reset watch %+v", changed)
    }
    // A reset inherits the most recently verified Docker tag.
    if err := s.SetServiceWatchOverride(ctx, "app", "web", nil); err != nil { t.Fatal(err) }
    if got := getOnePolicy(t, s).Image; got != "registry-1.docker.io/library/nginx:1.2.4" {
        t.Fatalf("reset did not inherit latest deployed tag %s", got)
    }
}

func TestWatchOverrideRejectsUnsafeOrUnavailableTargets(t *testing.T) {
    ctx := context.Background()
    s := newLabelTestStore(t)
    c := testLabelContainer("app", "web", "nginx:latest", map[string]string{labelMonitor:"true"})
    doSnapshot(t, s, c)
    for _, bad := range []string{
        "", "nginx", "nginx@sha256:" + strings.Repeat("a", 64),
        "example.com/other/nginx:stable", "other:latest", "nginx:latest ",
        "https://nginx:latest", "user:secret@nginx:stable", "nginx:!!!",
    } {
        t.Run(bad, func(t *testing.T) {
            if err := s.SetServiceWatchOverride(ctx, "app", "web", &bad); err == nil {
                t.Fatalf("accepted invalid watch target %q", bad)
            }
            if got := getOnePolicy(t, s).Image; got != observedNginx {
                t.Fatalf("bad target changed policy: %s", got)
            }
        })
    }
    if err := s.SetServiceWatchOverride(ctx, "missing", "web", nil); err == nil {
        t.Fatal("accepted unregistered service")
    }
    c.MonitorLabels[labelMonitor] = "false"
    doSnapshot(t, s, c)
    tag := "nginx:stable"
    if err := s.SetServiceWatchOverride(ctx, "app", "web", &tag); err == nil {
        t.Fatal("explicit opt-out failed to block watch edit")
    }
    if err := s.SetServiceWatchOverride(ctx, "app", "web", nil); err == nil {
        t.Fatal("explicit opt-out failed to block reset")
    }
    if len(dueAt(t, s, time.Now().Add(time.Minute))) != 0 {
        t.Fatal("disabled service remained due")
    }
    c.MonitorLabels = nil
    doSnapshot(t, s, c)
    if err := s.SetServiceWatchOverride(ctx, "app", "web", &tag); err == nil {
        t.Fatal("unenrolled service accepted watch edit")
    }
    c.MonitorLabels = map[string]string{labelMonitor:"true"}
    c.ImageArchitecture = ""
    doSnapshot(t, s, c)
    if err := s.SetServiceWatchOverride(ctx, "app", "web", &tag); err == nil {
        t.Fatal("unverified platform accepted watch edit")
    }
}

func TestWatchOverrideRepositoryDriftFailsClosed(t *testing.T) {
    ctx := context.Background()
    s := newLabelTestStore(t)
    c := testLabelContainer("app", "web", "nginx:stable", map[string]string{labelMonitor:"true"})
    doSnapshot(t, s, c)
    target := "nginx:latest"
    if err := s.SetServiceWatchOverride(ctx, "app", "web", &target); err != nil { t.Fatal(err) }
    old := getOnePolicy(t, s)
    c.Image = "redis:latest"
    doSnapshot(t, s, c)
    current := getOnePolicy(t, s)
    if current.Image != old.Image || len(dueAt(t,s,time.Now().Add(time.Hour))) != 0 {
        t.Fatal("external repository replacement bypassed watch safety")
    }
    views, err := s.MonitoringServiceSettings(ctx)
    if err != nil || views[0].DiscoveryState != "conflict" ||
        views[0].WatchImageOverride == nil || views[0].ValidationError == "" {
        t.Fatalf("missing visible watch drift conflict %+v %v",views,err)
    }
    if err := s.FinishMonitorCheck(ctx, old, time.Now().UTC(),nil);err!=nil{t.Fatal(err)}
    if getOnePolicy(t, s).LastCheckedAt != "" { t.Fatal("old check committed after conflict") }
    c.Image = "nginx:1.9.0"
    doSnapshot(t, s, c)
    if p := getOnePolicy(t, s); p.Image != observedNginx ||
        len(dueAt(t,s,time.Now().Add(time.Minute))) != 1 {
        t.Fatalf("recovered same repository lost watch %+v",p)
    }
}

func TestGUIWatchOverrideSurvivesDisableReenableAndRestart(t *testing.T) {
    ctx := context.Background()
    path := filepath.Join(t.TempDir(), "watch.db")
    s, err := Open(path)
    if err != nil { t.Fatal(err) }
    c := testLabelContainer("project", "web", "nginx:latest", nil)
    doSnapshot(t, s, c)
    target := "nginx:stable"
    if err := s.SetServiceWatchOverride(ctx, c.Project, c.Service, &target); err == nil {
        t.Fatal("watch edit enrolled service")
    }
    if err := s.SetServiceGUIEnrollment(ctx, c.Project, c.Service, true); err != nil { t.Fatal(err) }
    if err := s.SetServiceWatchOverride(ctx, c.Project, c.Service, &target); err != nil { t.Fatal(err) }
    if err := s.SetServiceGUIEnrollment(ctx, c.Project, c.Service, false); err != nil { t.Fatal(err) }
    if len(dueAt(t,s,time.Now().Add(time.Minute))) != 0 {t.Fatal("disabled watch still active")}
    if err := s.Close(); err != nil {t.Fatal(err)}
    s, err = Open(path)
    if err != nil {t.Fatal(err)}
    defer s.Close()
    config, err := s.MonitoringServiceSettings(ctx)
    if err != nil || len(config) != 1 || config[0].WatchImageOverride == nil ||
        *config[0].WatchImageOverride != watchedStable {
        t.Fatalf("override not durable %+v %v",config,err)
    }
    if err := s.SetServiceGUIEnrollment(ctx, c.Project, c.Service, true); err != nil {t.Fatal(err)}
    if p := getOnePolicy(t,s);p.Image != watchedStable {t.Fatalf("reenrollment lost override %+v",p)}
    if err := s.SetServiceWatchOverride(ctx,c.Project,c.Service,nil);err!=nil{t.Fatal(err)}
    if p := getOnePolicy(t,s);p.Image != observedNginx {t.Fatal("reset after restart wrong")}
}

func TestWatchOverrideDoesNotDisturbSharedRegistryHistory(t *testing.T) {
    ctx := context.Background()
    s := newLabelTestStore(t)
    a := testLabelContainer("a", "web", "nginx:latest", map[string]string{labelMonitor:"true"})
    b := testLabelContainer("b", "web", "nginx:latest", map[string]string{labelMonitor:"true"})
    doSnapshot(t,s,a,b)
    if _,err:=s.RecordRegistryCheck(ctx,observedNginx,"linux/amd64",
        "sha256:"+strings.Repeat("a",64),"sha256:"+strings.Repeat("b",64));err!=nil{t.Fatal(err)}
    before,err:=s.RegistryChecks(ctx)
    if err!=nil||len(before)!=1{t.Fatalf("missing baseline %+v %v",before,err)}
    target:="nginx:stable"
    if err:=s.SetServiceWatchOverride(ctx,"a","web",&target);err!=nil{t.Fatal(err)}
    policies,err:=s.MonitorPolicies(ctx)
    if err!=nil||len(policies)!=2||policies[0].Image!=watchedStable||policies[1].Image!=observedNginx {
        t.Fatalf("shared watches changed %+v %v",policies,err)
    }
    after,err:=s.RegistryChecks(ctx)
    if err!=nil||len(after)!=1||after[0].Image!=before[0].Image||after[0].CheckedAt!=before[0].CheckedAt {
        t.Fatalf("watch change corrupted shared observation %+v %v",after,err)
    }
    events,err:=s.Events(ctx,20)
    if err!=nil||len(events)!=0{t.Fatalf("watch change fabricated registry events %+v %v",events,err)}
}

func TestLegacyWatchOverrideRejectedAndUpgradeDataPreserved(t *testing.T) {
    ctx:=context.Background()
    path:=filepath.Join(t.TempDir(),"old.db")
    makeBeta2Fixture(t,path)
    s,err:=Open(path)
    if err!=nil{t.Fatal(err)}
    defer s.Close()
    if v:=versionOf(t,s.db);v!=4{t.Fatalf("expected schema v4, got %d",v)}
    settings,err:=s.MonitoringServiceSettings(ctx)
    if err!=nil||len(settings)!=2{t.Fatalf("lost provenance %+v %v",settings,err)}
    for _,m:=range settings {
        if !m.LegacyEnrolled||m.WatchImageOverride!=nil {
            t.Fatalf("legacy provenance overridden %+v",m)
        }
    }
    old,err:=s.MonitorPolicies(ctx)
    if err!=nil {t.Fatal(err)}
    target:="nginx:stable"
    if err:=s.SetServiceWatchOverride(ctx,old[0].Project,old[0].Service,&target);err==nil {
        t.Fatal("unexpected legacy override support")
    }
    updated,err:=s.MonitorPolicies(ctx)
    if err!=nil||len(updated)!=len(old) {
        t.Fatalf("lost legacy policies %+v %v",updated,err)
    }
    for i:=range old {
        if updated[i].Image!=old[i].Image || updated[i].Paused!=old[i].Paused ||
            updated[i].LastCheckedAt!=old[i].LastCheckedAt ||
            updated[i].ConsecutiveFailures!=old[i].ConsecutiveFailures {
            t.Fatalf("legacy policy lost history before=%+v after=%+v",old[i],updated[i])
        }
    }
}
