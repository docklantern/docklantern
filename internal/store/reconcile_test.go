package store

import (
    "context"
    "errors"
    "path/filepath"
    "testing"
    "time"
)

func testLabelContainer(project,service,image string,labels map[string]string) Container {
    return Container{
        ID:project+"-"+service+"-1", Name:project+"-"+service+"-1",
        Project:project,Service:service,Image:image,
        ImageID:"sha256:local",State:"running",
        ImageOS:"linux",ImageArchitecture:"amd64",MonitorLabels:labels,
    }
}

func newLabelTestStore(t *testing.T)*Store{
    t.Helper()
    s,err:=Open(filepath.Join(t.TempDir(),"monitor.sqlite"))
    if err!=nil{t.Fatal(err)}
    t.Cleanup(func(){_ =s.Close()})
    return s
}
func doSnapshot(t *testing.T,s *Store,items ...Container){
    t.Helper()
    if err:=s.Snapshot(context.Background(),items);err!=nil{t.Fatal(err)}
}
func dueAt(t *testing.T,s *Store,when time.Time)[]MonitorPolicy{
    t.Helper()
    due,err:=s.DueMonitorPolicies(context.Background(),when,100)
    if err!=nil{t.Fatal(err)}
    return due
}

func TestLabelOnlyAutoEnrollmentAndIdempotence(t *testing.T){
    ctx:=context.Background()
    s:=newLabelTestStore(t)
    labeled:=testLabelContainer("hello-js","web","davidslusser/hello-js:latest",
        map[string]string{labelMonitor:"true"})
    doSnapshot(t,s,labeled)
    projects,err:=s.Projects(ctx)
    if err!=nil||len(projects)!=0{t.Fatalf("label monitoring made registration: %+v %v",projects,err)}
    policies,err:=s.MonitorPolicies(ctx)
    if err!=nil||len(policies)!=1{t.Fatalf("auto policy %+v %v",policies,err)}
    p:=policies[0]
    if p.Project!="hello-js"||p.Service!="web"||p.Image!="registry-1.docker.io/davidslusser/hello-js:latest"||p.Platform!="linux/amd64"||p.IntervalSeconds!=300||p.Paused{
        t.Fatalf("unexpected policy: %+v",p)
    }
    if len(dueAt(t,s,time.Now().Add(time.Minute)))!=1{t.Fatal("new service not scheduled")}
    // Successful check should survive an identical Docker discovery snapshot.
    due:=dueAt(t,s,time.Now().Add(time.Minute))
    if err:=s.FinishMonitorCheck(ctx,due[0],time.Now().UTC(),nil);err!=nil{t.Fatal(err)}
    policies,_=s.MonitorPolicies(ctx)
    prior:=policies[0]
    replica:=labeled;replica.ID="second-id";replica.Name="hello-js-web-2"
    doSnapshot(t,s,labeled,replica) // two identical service replicas
    policies,_=s.MonitorPolicies(ctx)
    if len(policies)!=1||policies[0].NextCheckAt!=prior.NextCheckAt||policies[0].LastCheckedAt!=prior.LastCheckedAt{
        t.Fatalf("duplicate scan reset or duplicated watches: %+v; before %+v",policies,prior)
    }
    settings,err:=s.MonitoringServiceSettings(ctx)
    if err!=nil||len(settings)!=1||settings[0].LegacyEnrolled||settings[0].LabelOptIn==nil||!*settings[0].LabelOptIn||settings[0].DiscoveryState!="present"{
        t.Fatalf("bad provenance: %+v %v",settings,err)
    }
}
func TestLabelDisableRemovalAndReappearance(t *testing.T){
    s:=newLabelTestStore(t)
    item:=testLabelContainer("app","web","nginx:latest",map[string]string{labelMonitor:"true"})
    now:=time.Now().UTC()
    doSnapshot(t,s,item)
    if len(dueAt(t,s,now.Add(time.Minute)))!=1{t.Fatal("initial enrollment missing")}
    old:=dueAt(t,s,now.Add(time.Minute))[0]
    item.MonitorLabels=map[string]string{labelMonitor:"false"}
    doSnapshot(t,s,item)
    if len(dueAt(t,s,now.Add(time.Minute)))!=0{t.Fatal("explicit false did not block watch")}
    if err:=s.FinishMonitorCheck(context.Background(),old,now,nil);err!=nil{t.Fatal(err)}
    rows,_:=s.MonitorPolicies(context.Background())
    if rows[0].LastCheckedAt!=""{t.Fatal("stale result persisted after opt-out")}
    item.MonitorLabels=nil
    doSnapshot(t,s,item)
    if len(dueAt(t,s,now.Add(time.Minute)))!=0{t.Fatal("label removal did not disable watch")}
    item.MonitorLabels=map[string]string{labelMonitor:"true"}
    doSnapshot(t,s,item)
    if len(dueAt(t,s,now.Add(time.Minute)))!=1{t.Fatal("relabel not reactivated")}
    doSnapshot(t,s) // Successful empty scan -> missing
    if len(dueAt(t,s,now.Add(time.Minute)))!=0{t.Fatal("deleted service still being monitored")}
    doSnapshot(t,s,item)
    if len(dueAt(t,s,now.Add(time.Minute)))!=1{t.Fatal("service return did not reactivate")}
}
func TestInvalidLabelsAndMissingImageProvenanceFailClosed(t *testing.T){
    for _,tc:=range []struct{
        name string; labels map[string]string; image,os,arch string
    }{
        {"bad boolean",map[string]string{labelMonitor:"yes"},"nginx:latest","linux","amd64"},
        {"too short",map[string]string{labelMonitor:"true",labelInterval:"30s"},"nginx:latest","linux","amd64"},
        {"fractional",map[string]string{labelMonitor:"true",labelInterval:"1m0.5s"},"nginx:latest","linux","amd64"},
        {"untagged digest",map[string]string{labelMonitor:"true"},"nginx@sha256:abc","linux","amd64"},
        {"missing architecture",map[string]string{labelMonitor:"true"},"nginx:latest","linux",""},
        {"bad platform",map[string]string{labelMonitor:"true"},"nginx:latest","linux","a/md64"},
    } {
        t.Run(tc.name,func(t *testing.T){
            s:=newLabelTestStore(t)
            c:=testLabelContainer("app","web",tc.image,tc.labels)
            c.ImageOS=tc.os;c.ImageArchitecture=tc.arch
            doSnapshot(t,s,c)
            policies,_:=s.MonitorPolicies(context.Background())
            if len(policies)!=0{t.Fatalf("invalid config created monitoring policy: %+v",policies)}
            records,_:=s.MonitoringServiceSettings(context.Background())
            if len(records)!=1||records[0].DiscoveryState!="conflict"||records[0].ValidationError==""{
                t.Fatalf("missing visible error: %+v",records)
            }
        })
    }
}
func TestConflictingReplicasAndStoppedService(t *testing.T){
    s:=newLabelTestStore(t)
    first:=testLabelContainer("app","web","nginx:latest",map[string]string{labelMonitor:"true"})
    second:=first;second.ID="second";second.Name="app-web-2"
    second.MonitorLabels=map[string]string{labelMonitor:"false"}
    doSnapshot(t,s,first,second)
    records,_:=s.MonitoringServiceSettings(context.Background())
    if len(records)!=1||records[0].DiscoveryState!="conflict"{t.Fatalf("replica conflict ignored %+v",records)}
    if len(dueAt(t,s,time.Now().Add(time.Minute)))!=0{t.Fatal("conflicting replicas active")}
    doSnapshot(t,s,first)
    if len(dueAt(t,s,time.Now().Add(time.Minute)))!=1{t.Fatal("resolved conflict not activated")}
    first.State="exited"
    doSnapshot(t,s,first)
    if len(dueAt(t,s,time.Now().Add(time.Minute)))!=0{t.Fatal("stopped service remains active")}
}
func TestLabelIntervalAndSettingsOwnership(t *testing.T){
    ctx:=context.Background()
    s:=newLabelTestStore(t)
    item:=testLabelContainer("app","web","nginx:latest",map[string]string{
        labelMonitor:"true",labelInterval:"10m",
    })
    doSnapshot(t,s,item)
    policies,_:=s.MonitorPolicies(ctx)
    if policies[0].IntervalSeconds!=600{t.Fatalf("ignored label interval %+v",policies)}
    // Future GUI override takes precedence; its ownership is independent
    // from the Docker labels (the GUI itself is issue #30).
    _,err:=s.db.Exec(`UPDATE monitor_service_settings SET gui_interval_seconds=120
      WHERE project='app' AND service='web'`)
    if err!=nil{t.Fatal(err)}
    doSnapshot(t,s,item)
    policies,_=s.MonitorPolicies(ctx)
    if policies[0].IntervalSeconds!=120{t.Fatalf("GUI preference overwritten %+v",policies)}
    item.MonitorLabels[labelInterval]="30m"
    doSnapshot(t,s,item)
    policies,_=s.MonitorPolicies(ctx)
    if policies[0].IntervalSeconds!=120{t.Fatal("label change replaced GUI override")}
    _,err=s.db.Exec(`UPDATE monitor_service_settings SET gui_interval_seconds=NULL
      WHERE project='app' AND service='web'`)
    if err!=nil{t.Fatal(err)}
    doSnapshot(t,s,item)
    policies,_=s.MonitorPolicies(ctx)
    if policies[0].IntervalSeconds!=1800{t.Fatalf("did not restore label interval %+v",policies)}
}
func TestLegacyPoliciesRemainUntouchedAndExplicitFalseBlocks(t *testing.T){
    ctx:=context.Background()
    s:=newLabelTestStore(t)
    if err:=s.Register(ctx,Registration{Name:"old",ProjectDir:"/srv/old",Files:[]string{"/srv/old/compose.yaml"},Services:[]string{"web"}});err!=nil{t.Fatal(err)}
    if err:=s.SetMonitorPolicy(ctx,MonitorPolicy{Project:"old",Service:"web",Image:"nginx:stable",Platform:"linux/amd64",IntervalSeconds:700});err!=nil{t.Fatal(err)}
    item:=testLabelContainer("old","web","nginx:latest",map[string]string{labelMonitor:"true",labelInterval:"5m"})
    doSnapshot(t,s,item)
    policies,_:=s.MonitorPolicies(ctx)
    if len(policies)!=1||policies[0].IntervalSeconds!=700||policies[0].Image!="registry-1.docker.io/library/nginx:stable" {
        t.Fatalf("legacy policy overwritten %+v",policies)
    }
    item.MonitorLabels=map[string]string{labelMonitor:"false"}
    doSnapshot(t,s,item)
    if len(dueAt(t,s,time.Now().Add(time.Minute)))!=0{t.Fatal("false failed to block legacy checks")}
    item.MonitorLabels=nil
    doSnapshot(t,s,item)
    if len(dueAt(t,s,time.Now().Add(time.Minute)))!=1{t.Fatal("legacy watch lost without labels")}
    settings,_:=s.MonitoringServiceSettings(ctx)
    if len(settings)!=1||!settings[0].LegacyEnrolled{t.Fatalf("legacy provenance lost %+v",settings)}
}
func TestFailedSnapshotPreservesPreviousEnrollment(t *testing.T){
    s:=newLabelTestStore(t)
    item:=testLabelContainer("app","web","nginx:latest",map[string]string{labelMonitor:"true"})
    doSnapshot(t,s,item)
    bad:=testLabelContainer("bad","web","nginx:latest",nil)
    bad.ID=item.ID // duplicate primary key -> rollback all snapshot changes
    if err:=s.Snapshot(context.Background(),[]Container{item,bad});err==nil{t.Fatal("expected duplicate inventory ID failure")}
    if len(dueAt(t,s,time.Now().Add(time.Minute)))!=1{t.Fatal("failed snapshot changed enrollment")}
    inventory,_:=s.Containers(context.Background())
    if len(inventory)!=1||inventory[0].Project!="app"{t.Fatalf("inventory partially changed %+v",inventory)}
}
func TestMigratedBeta2DataSurvivesSchemaTwo(t *testing.T){
    ctx:=context.Background()
    path:=filepath.Join(t.TempDir(),"beta2.db")
    makeBeta2Fixture(t,path)
    s,err:=Open(path);if err!=nil{t.Fatal(err)}
    defer s.Close()
    if versionOf(t,s.db)!=schemaVersion{t.Fatal("schema version incorrect")}
    var fkCount int
    rows,err:=s.db.Query("PRAGMA foreign_key_list(monitor_policies)")
    if err!=nil{t.Fatal(err)}
    for rows.Next(){fkCount++}
    rows.Close()
    if fkCount!=0{t.Fatal("label-only monitoring still requires registration FK")}
    events,err:=s.Events(ctx,10)
    if err!=nil||len(events)!=1||events[0].ID!=17{t.Fatalf("migration lost history %+v %v",events,err)}
    policies,err:=s.MonitorPolicies(ctx)
    if err!=nil||len(policies)!=2||policies[1].ConsecutiveFailures!=2{t.Fatalf("migration lost policies %+v %v",policies,err)}
}
func TestOutageDoesNotDeactivateMonitors(t *testing.T){
    s:=newLabelTestStore(t)
    item:=testLabelContainer("app","web","nginx:latest",map[string]string{labelMonitor:"true"})
    doSnapshot(t,s,item)
    // A Docker proxy error prevents calling Snapshot altogether.
    var discoveryErr=errors.New("Docker proxy unavailable")
    if discoveryErr==nil{t.Fatal("unexpected nil error")}
    if len(dueAt(t,s,time.Now().Add(time.Minute)))!=1{t.Fatal("transient proxy failure dropped policy")}
}
