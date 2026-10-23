package updates
import(
 "strings"
 "testing"
 "github.com/docklantern/docklantern/internal/store"
)
func TestAssessmentEvidence(t *testing.T){
 a:="sha256:"+strings.Repeat("a",64)
 b:="sha256:"+strings.Repeat("b",64)
 projects:=[]store.Registration{{Name:"site",Services:[]string{"web","worker"}}}
 policy:=store.MonitorPolicy{Project:"site",Service:"web",Image:"registry-1.docker.io/library/nginx:latest",Platform:"linux/amd64"}
 checks:=[]store.RegistryCheck{{Image:policy.Image,Platform:policy.Platform,PlatformDigest:a,IndexDigest:b}}
 cases:=[]struct{name,image,state string}{
  {"tag only","nginx:latest","unknown"},
  {"config id, not digest","nginx:latest","unknown"},
  {"correct pinned","nginx@"+a,"current"},
  {"matching root index","docker.io/library/nginx@"+b,"current"},
  {"different pinned","nginx@sha256:"+strings.Repeat("c",64),"different"},
  {"wrong repo","redis@"+a,"unknown"},
  {"fake sha","nginx@sha256:blah","unknown"},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   cs:=[]store.Container{{Project:"site",Service:"web",Image:tc.image,ImageID:a,State:"running"}}
   got:=Assess(projects,cs,[]store.MonitorPolicy{policy},checks)
   if len(got)!=1||got[0].State!=tc.state{t.Fatalf("%+v",got)}
  })
 }
 mixed:=Assess(projects,[]store.Container{{Project:"site",Service:"web",Image:"nginx@"+a},{Project:"site",Service:"web",Image:"nginx@sha256:"+strings.Repeat("c",64)}},[]store.MonitorPolicy{policy},checks)
 if mixed[0].State!="unknown"{t.Fatalf("mixed replicas: %+v",mixed)}
}
func TestUnverifiedStates(t *testing.T){
 p:=store.MonitorPolicy{Project:"app",Service:"web",Image:"ghcr.io/org/web:latest",Platform:"linux/amd64"}
 cases:=[]struct{projects []store.Registration;checks []store.RegistryCheck;state string}{
  {nil,nil,"unregistered"},
  {[]store.Registration{{Name:"app",Services:[]string{"web"}}},nil,"not_checked"},
  {[]store.Registration{{Name:"app",Services:[]string{"web"}}},[]store.RegistryCheck{{Image:p.Image,Platform:p.Platform,PlatformDigest:"sha256:abc"}},"not_running"},
 }
 for _,tc:=range cases{
  got:=Assess(tc.projects,nil,[]store.MonitorPolicy{p},tc.checks)
  if len(got)!=1||got[0].State!=tc.state{t.Fatalf("%+v",got)}
 }
}
