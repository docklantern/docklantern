package executorpreflight

import (
 "encoding/json"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/executorregistration"
)

func fixtureRegistration() executorregistration.Registration {
 uid:=uint32(0)
 return executorregistration.Registration{
  SchemaVersion:1,RegistrationID:"registered-demo",ProjectName:"hello-project",
  ProjectDirectory:"/srv/hello",ComposeFiles:[]string{"/srv/hello/compose.yaml"},
  TrustedSourceUID:&uid,FeaturePolicy:executorregistration.FeaturePolicyPrebuilt,
  Services:[]executorregistration.Service{{Name:"web",DeploymentEnabled:true,
    ApprovedRepository:"docker.io/example/hello-go",
    Platform:executorregistration.Platform{OS:"linux",Architecture:"amd64"}}},
 }
}

func configs()(map[string]any,map[string]any,string){
 digest:="sha256:"+strings.Repeat("a",64)
 a:=map[string]any{
  "name":"hello-project","networks":map[string]any{"web":map[string]any{"external":true}},
  "services":map[string]any{
   "web":map[string]any{"image":"docker.io/example/hello-go:stable",
    "labels":map[string]any{"traefik.enable":"true"},
    "ports":[]any{map[string]any{"published":8080,"target":8080}},
    "environment":map[string]any{"MODE":"prod"}},
   "worker":map[string]any{"image":"docker.io/example/worker:latest"},
  },
 }
 clone:=func(m map[string]any)map[string]any{
  b,_:=json.Marshal(m)
  var out map[string]any
  _=json.Unmarshal(b,&out)
  return out
 }
 b:=clone(a)
 b["services"].(map[string]any)["web"].(map[string]any)["image"]="docker.io/example/hello-go@"+digest
 return a,b,digest
}
func raw(m map[string]any)[]byte{b,_:=json.Marshal(m);return b}

func TestImageOnlyExactDiff(t *testing.T){
 a,b,digest:=configs()
 if e:=CheckImageOnly(raw(a),raw(b),fixtureRegistration(),"web","docker.io/example/hello-go",digest);e!=nil{
  t.Fatal("exact image-only change rejected",e)
 }
}

func TestImageOnlyRejectsAnyOtherChange(t *testing.T){
 cases:=[]struct{name string;change func(map[string]any)}{
  {"sibling_image",func(b map[string]any){b["services"].(map[string]any)["worker"].(map[string]any)["image"]="docker.io/example/worker:new"}},
  {"port",func(b map[string]any){b["services"].(map[string]any)["web"].(map[string]any)["ports"]=[]any{map[string]any{"published":9000,"target":8080}}}},
  {"label",func(b map[string]any){b["services"].(map[string]any)["web"].(map[string]any)["labels"]=map[string]any{"traefik.enable":"false"}}},
  {"env",func(b map[string]any){b["services"].(map[string]any)["web"].(map[string]any)["environment"]=map[string]any{"MODE":"test"}}},
  {"network",func(b map[string]any){b["networks"]=map[string]any{"web":map[string]any{"external":false}}}},
  {"volume",func(b map[string]any){b["services"].(map[string]any)["web"].(map[string]any)["volumes"]=[]any{"data:/data"}}},
  {"new_service",func(b map[string]any){b["services"].(map[string]any)["extra"]=map[string]any{"image":"docker.io/example/extra"}}},
  {"build",func(b map[string]any){b["services"].(map[string]any)["web"].(map[string]any)["build"]="."}},
  {"privileged",func(b map[string]any){b["services"].(map[string]any)["web"].(map[string]any)["privileged"]=true}},
  {"depends_on",func(b map[string]any){b["services"].(map[string]any)["web"].(map[string]any)["depends_on"]=[]any{"worker"}}},
  {"socket_mount",func(b map[string]any){b["services"].(map[string]any)["web"].(map[string]any)["volumes"]=[]any{"/var/run/docker.sock:/var/run/docker.sock"}}},
  {"wrong_image",func(b map[string]any){b["services"].(map[string]any)["web"].(map[string]any)["image"]="docker.io/example/hello-go:latest"}},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  a,b,digest:=configs()
  tc.change(b)
  if e:=CheckImageOnly(raw(a),raw(b),fixtureRegistration(),"web","docker.io/example/hello-go",digest);e==nil{
   t.Fatal("unsupported Compose modification passed")
  }
 })}
}

func TestImageOnlyRejectsUntrustedIdentityAndInput(t *testing.T){
 a,b,digest:=configs()
 reg:=fixtureRegistration()
 bad:=[]struct{name,svc,repo,hash string}{
  {"wrong_service","worker","docker.io/example/hello-go",digest},
  {"wrong_repository","web","docker.io/example/other",digest},
  {"wrong_digest","web","docker.io/example/hello-go","sha256:"+strings.Repeat("A",64)},
  {"short_digest","web","docker.io/example/hello-go","sha256:abc"},
 }
 for _,tc:=range bad{t.Run(tc.name,func(t *testing.T){
  if e:=CheckImageOnly(raw(a),raw(b),reg,tc.svc,tc.repo,tc.hash);e==nil{t.Fatal("unapproved target accepted")}
 })}
 reg.Services[0].DeploymentEnabled=false
 if e:=CheckImageOnly(raw(a),raw(b),reg,"web","docker.io/example/hello-go",digest);e==nil{t.Fatal("disabled registration accepted")}
 for _,tc:=range [][]byte{nil,[]byte("{}"),[]byte("not JSON"),[]byte("{}{}"),[]byte(strings.Repeat("x",MaxConfigBytes+1))}{
  if e:=CheckImageOnly(tc,raw(b),fixtureRegistration(),"web","docker.io/example/hello-go",digest);e==nil{t.Fatal("invalid normalized source accepted")}
 }
}
