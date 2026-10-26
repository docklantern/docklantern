package updates

import (
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/store"
)

func TestLabelOnlyAssessmentNeedsNoComposeRegistration(t *testing.T){
 now:=time.Now().UTC().Truncate(time.Second)
 digest:="sha256:"+strings.Repeat("a",64)
 remote:="ghcr.io/acme/web:latest"
 policies:=[]store.MonitorPolicy{{Project:"label-project",Service:"web",Image:remote,Platform:"linux/amd64"}}
 containers:=[]store.Container{{
  Project:"label-project",Service:"web",
  Image:remote,ImageOS:"linux",ImageArchitecture:"amd64",
  RepoDigests:[]string{"ghcr.io/acme/web@"+digest},State:"running",
  ObservedAt:now.Format(time.RFC3339),
 }}
 checks:=[]store.RegistryCheck{{Image:remote,Platform:"linux/amd64",PlatformDigest:digest,IndexDigest:digest,CheckedAt:now.Format(time.RFC3339)}}
 legacy:=AssessFresh(nil,containers,policies,checks,now)
 if len(legacy)!=1||legacy[0].State!="unregistered"{t.Fatalf("legacy contract changed %+v",legacy)}
 monitored:=AssessMonitoredFresh(nil,containers,policies,checks,now)
 if len(monitored)!=1||monitored[0].State!="current"||monitored[0].ReleaseClassification!="not_verified"{
  t.Fatalf("label-only image evidence lost: %+v",monitored)
 }
}
