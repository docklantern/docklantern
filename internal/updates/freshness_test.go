package updates

import (
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/store"
)

func TestAssessFreshRequiresRecentIndependentObservations(t *testing.T){
 now:=time.Date(2026,10,22,16,0,0,0,time.UTC)
 digest:="sha256:"+strings.Repeat("a",64)
 image:="ghcr.io/example/app:latest"
 projects:=[]store.Registration{{Name:"app",Services:[]string{"web"}}}
 policy:=[]store.MonitorPolicy{{Project:"app",Service:"web",Image:image,Platform:"linux/amd64"}}
 base:=store.Container{Project:"app",Service:"web",Image:"ghcr.io/example/app@"+digest,ImageOS:"linux",ImageArchitecture:"amd64",State:"running"}
 registry:=store.RegistryCheck{Image:image,Platform:"linux/amd64",IndexDigest:digest,PlatformDigest:digest}
 cases:=[]struct{name string;inventory,remote time.Time;want string}{
  {"fresh",now.Add(-time.Minute),now.Add(-time.Minute),"current"},
  {"stale inventory",now.Add(-6*time.Minute),now.Add(-time.Minute),"stale_inventory"},
  {"stale registry",now.Add(-time.Minute),now.Add(-3*time.Hour),"stale_registry"},
  {"future registry",now.Add(-time.Minute),now.Add(time.Minute),"stale_registry"},
  {"future inventory",now.Add(time.Minute),now.Add(-time.Minute),"stale_inventory"},
  {"inventory boundary",now.Add(-5*time.Minute),now.Add(-time.Minute),"current"},
  {"registry boundary",now.Add(-time.Minute),now.Add(-2*time.Hour),"current"},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   c:=base;c.ObservedAt=tc.inventory.Format(time.RFC3339)
   remote:=registry;remote.CheckedAt=tc.remote.Format(time.RFC3339)
   assessments:=AssessFresh(projects,[]store.Container{c},policy,[]store.RegistryCheck{remote},now)
   if len(assessments)!=1||assessments[0].State!=tc.want {t.Fatalf("%+v",assessments)}
   if assessments[0].ReleaseClassification!="not_verified"{t.Fatalf("unexpected release classification: %+v",assessments[0])}
  })
 }
}
func TestAssessFreshMissingEvidenceAndReplicas(t *testing.T){
 now:=time.Now().UTC().Truncate(time.Second)
 digest:="sha256:"+strings.Repeat("c",64)
 image:="ghcr.io/example/app:latest"
 projects:=[]store.Registration{{Name:"app",Services:[]string{"web"}}}
 policies:=[]store.MonitorPolicy{{Project:"app",Service:"web",Image:image,Platform:"linux/amd64"}}
 checks:=[]store.RegistryCheck{{Image:image,Platform:"linux/amd64",CheckedAt:now.Format(time.RFC3339),PlatformDigest:digest}}
 replicas:=[]store.Container{
  {Project:"app",Service:"web",Image:"ghcr.io/example/app@"+digest,ObservedAt:now.Format(time.RFC3339)},
  {Project:"app",Service:"web",Image:"ghcr.io/example/app@"+digest},
 }
 result:=AssessFresh(projects,replicas,policies,checks,now)
 if result[0].State!="stale_inventory"{t.Fatalf("%+v",result)}
 checks[0].CheckedAt=""
 result=AssessFresh(projects,replicas,policies,checks,now)
 if result[0].State!="stale_registry"{t.Fatalf("%+v",result)}
}
