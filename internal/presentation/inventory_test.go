package presentation

import (
    "reflect"
    "testing"
    "time"

    "github.com/docklantern/docklantern/internal/store"
    "github.com/docklantern/docklantern/internal/updates"
)

func TestImageIDRoundTripAndInvalid(t *testing.T) {
    for _,id:=range []ImageIdentity{
        {"docker.io/library/nginx:latest","linux/amd64"},
        {"ghcr.io/acme/backend:v1@sha256:abcd","linux/arm64/v8"},
    } {
        got,err:=ParseImageID(ImageID(id))
        if err!=nil || got!=id {t.Fatalf("round trip for %+v: %+v %v",id,got,err)}
    }
    for _,bad:=range []string{"","%%%","YWJj","YQBiAGM","YXw","YQBiAA"}{
        if _,err:=ParseImageID(bad);err==nil{t.Errorf("accepted malformed image ID %q",bad)}
    }
}

func TestBuildImageSummariesGroupsAndPreservesHistory(t *testing.T){
    now:=time.Date(2026,10,26,12,0,0,0,time.UTC)
    img:="docker.io/library/nginx:latest"
    other:="docker.io/library/nginx:stable"
    policies:=[]store.MonitorPolicy{
        {Project:"site",Service:"web",Image:img,Platform:"linux/amd64"},
        {Project:"edge",Service:"web",Image:img,Platform:"linux/amd64"},
        {Project:"arm",Service:"web",Image:img,Platform:"linux/arm64"},
        {Project:"site",Service:"worker",Image:other,Platform:"linux/amd64"},
    }
    checks:=[]store.RegistryCheck{{Image:img,Platform:"linux/amd64",Status:"unchanged",CheckedAt:now.Add(-time.Minute).Format(time.RFC3339)}}
    assessments:=[]updates.Assessment{
        {Project:"site",Service:"web",State:"current"},
        {Project:"edge",Service:"web",State:"different"},
        {Project:"arm",Service:"web",State:"not_running"},
    }
    events:=[]store.Event{
        {Kind:"remote_digest_changed",Image:img,Platform:"linux/amd64",OccurredAt:now.Add(-time.Hour).Format(time.RFC3339)},
        {Kind:"remote_digest_changed",Image:img,Platform:"linux/amd64",OccurredAt:now.Add(-2*time.Hour).Format(time.RFC3339)},
        {Kind:"registry_failure",Image:other,Platform:"linux/amd64",OccurredAt:now.Format(time.RFC3339)},
    }
    got:=BuildImageSummaries(policies,checks,assessments,events,now)
    if len(got)!=3{t.Fatalf("want 3 reference/platform groups, got %+v",got)}
    var chosen *ImageSummary
    for i:=range got{
        if got[i].Reference==img && got[i].Platform=="linux/amd64"{chosen=&got[i]}
    }
    if chosen==nil{t.Fatal("missing expected image")}
    if chosen.RegistryStatus!="unchanged" || chosen.DeploymentStatus!="mixed"{
        t.Fatalf("registry and deployment independent: %+v",*chosen)
    }
    if chosen.LastChangeAt!=now.Add(-time.Hour).Format(time.RFC3339){
        t.Errorf("lost digest change after later unchanged registry observation: %s",chosen.LastChangeAt)
    }
    if chosen.LatestObservation==nil || chosen.LatestObservation.Status!="unchanged"{
        t.Fatal("latest successful registry evidence should remain present")
    }
    if chosen.DeploymentCounts.Current!=1 || chosen.DeploymentCounts.Different!=1{
        t.Fatalf("unexpected service assessment mix: %+v",chosen.DeploymentCounts)
    }
    if !reflect.DeepEqual(chosen.Services,[]ServiceIdentity{{"edge","web"},{"site","web"}}){
        t.Errorf("service identities: %+v",chosen.Services)
    }
}

func TestBuildImageSummariesCheckFailureKeepsLastSuccess(t *testing.T){
    now:=time.Date(2026,10,26,12,0,0,0,time.UTC)
    img:="ghcr.io/acme/app:main"
    p:=[]store.MonitorPolicy{{Project:"app",Service:"web",Image:img,Platform:"linux/amd64",LastError:"temporary registry error"}}
    c:=[]store.RegistryCheck{{Image:img,Platform:"linux/amd64",Status:"unchanged",CheckedAt:now.Add(-time.Hour).Format(time.RFC3339)}}
    got:=BuildImageSummaries(p,c,nil,nil,now)
    if len(got)!=1 || got[0].RegistryStatus!="check_failed" || got[0].FailedServices!=1 || got[0].LatestObservation==nil{
        t.Fatalf("failure should not remove last successful evidence: %+v",got)
    }
    p[0].LastError=""
    got=BuildImageSummaries(p,c,nil,nil,now.Add(4*time.Hour))
    if got[0].RegistryStatus!="stale"{t.Fatalf("old registry evidence should be stale: %+v",got[0])}
}

func TestSummarizeDeploymentsConservative(t *testing.T){
    cases:=[]struct{counts DeploymentCounts; want string}{
        {DeploymentCounts{},"unknown"},
        {DeploymentCounts{Current:2},"current"},
        {DeploymentCounts{Different:2},"different"},
        {DeploymentCounts{NotRunning:2},"not_running"},
        {DeploymentCounts{Unknown:2},"unknown"},
        {DeploymentCounts{Current:1,Unknown:1},"mixed"},
        {DeploymentCounts{Different:1,NotRunning:1},"mixed"},
        {DeploymentCounts{Unknown:1,NotRunning:1},"unknown"},
        {DeploymentCounts{Current:1,Different:1,Unknown:1},"mixed"},
    }
    for _,tc:=range cases{
        if got:=summarizeDeployments(tc.counts);got!=tc.want{
            t.Errorf("%+v: got %q, want %q",tc.counts,got,tc.want)
        }
    }
}

func TestBuildServiceSummariesIncludesUnmonitoredAndCapabilities(t *testing.T){
    no:=false
    views:=[]store.MonitorServiceView{
        {Project:"site",Service:"web",Image:"nginx:latest",Platform:"linux/amd64",EnrollmentSource:"Docker label",EffectiveInterval:600,IntervalSource:"Docker label",HasPolicy:true,CanPause:true,State:"present"},
        {Project:"site",Service:"cache",EnrollmentSource:"Not monitored",CanGUIEnable:true,State:"present"},
        {Project:"site",Service:"db",LabelOptIn:&no,EnrollmentSource:"Disabled by Docker label",State:"present"},
    }
    containers:=[]store.Container{
        {Project:"site",Service:"cache",Image:"redis:7",ImageOS:"linux",ImageArchitecture:"amd64",State:"running",Health:"healthy"},
        {Project:"site",Service:"web",Image:"nginx:latest",State:"running",Health:"healthy"},
        {Project:"site",Service:"web",Image:"nginx:latest",State:"running",Health:"healthy"},
    }
    got:=BuildServiceSummaries(views,containers,nil,nil)
    if len(got)!=3 || got[0].Service!="cache" || got[1].Service!="db" || got[2].Service!="web"{
        t.Fatalf("expected all discovered services sorted: %+v",got)
    }
    if got[0].Monitoring!="not_enrolled" || !got[0].CanGUIEnable || got[0].RunningCount!=1 || got[0].ImageID!=""{
        t.Fatalf("unenrolled service not represented correctly: %+v",got[0])
    }
    if got[1].Monitoring!="disabled_by_label" || got[1].CanGUIEnable{
        t.Fatalf("explicit monitor=false must remain disabled: %+v",got[1])
    }
    if got[2].Monitoring!="monitoring" || got[2].RunningCount!=2 || got[2].IntervalSeconds!=600 || !got[2].CanPause{
        t.Fatalf("monitored replicas/schedule/capabilities incorrect: %+v",got[2])
    }
}
