package executorpreflight

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/executorhost"
)

type hostFixtureRunner struct {
 calls []hostReadCommand
 response func(hostReadCommand)([]byte,error)
}
func (f *hostFixtureRunner) run(_ context.Context,c hostReadCommand)([]byte,error){
 f.calls=append(f.calls,c)
 if !approvedHostRead(c.args){return nil,ErrHostEvidenceUnavailable}
 if f.response==nil{return nil,ErrHostEvidenceUnavailable}
 return f.response(c)
}

func TestHostComposeConfigReadOnlyAndControlled(t *testing.T){
 reg:=fixtureRegistration()
 before,after,digest:=configs()
 r:=&hostFixtureRunner{response:func(c hostReadCommand)([]byte,error){
  if c.args[2]!="compose"{t.Fatal("unexpected Docker command",c.args)}
  if len(c.stdin)==0{return raw(before),nil}
  var m map[string]any
  if json.Unmarshal(c.stdin,&m)!=nil{t.Fatal("malformed candidate override")}
  svc:=m["services"].(map[string]any)["web"].(map[string]any)
  if svc["image"]!="docker.io/example/hello-go@"+digest{t.Fatal("candidate image not bound")}
  return raw(after),nil
 }}
 h:=HostReadOnlyAdapter{runner:r,clock:time.Now}
 got,err:=h.NormalizedCompose(context.Background(),reg,"web","")
 if err!=nil||!sameBytes(got,raw(before)){t.Fatalf("registered config failed: %v",err)}
 got,err=h.NormalizedCompose(context.Background(),reg,"web","docker.io/example/hello-go@"+digest)
 if err!=nil||!sameBytes(got,raw(after)){t.Fatalf("candidate config failed: %v",err)}
 if len(r.calls)!=2 {t.Fatalf("expected exact two read calls: %d",len(r.calls))}
 for _,call:=range r.calls{
  if call.args[0]!="--host"||call.args[1]!=executorhost.LocalSocket||
   call.args[2]!="compose"||call.args[3]!="--project-directory"||
   call.args[4]!=reg.ProjectDirectory||call.args[6]!=reg.ProjectName||
   call.args[len(call.args)-4]!="config"||
   call.args[len(call.args)-3]!="--no-env-resolution" {t.Fatalf("unsafe Compose invocation: %#v",call.args)}
 }
}

func TestHostAdapterRejectsCommandsAndConfigChanges(t *testing.T){
 for _,args:=range [][]string{
  {"--host",executorhost.LocalSocket,"pull","hello"},
  {"--host",executorhost.LocalSocket,"compose","up","-d"},
  {"--host",executorhost.LocalSocket,"inspect","--format","{{json .}}","../../bad"},
  {"--host","ssh://somehost","ps","--all"},
  {"--host",executorhost.LocalSocket,"compose","--project-directory","/srv/hello","-p","hello","-f","/srv/hello/compose.yaml","up","--format","json"},
 } {
  if approvedHostRead(args){t.Fatalf("mutating or remote command allowed: %#v",args)}
 }
 reg:=fixtureRegistration()
 bad:=[]string{
  "docker.io/example/other@sha256:"+strings.Repeat("a",64),
  "docker.io/example/hello-go:latest",
  "docker.io/example/hello-go@sha256:../../other",
 }
 for _,ref:=range bad{
  runner:=&hostFixtureRunner{response:func(c hostReadCommand)([]byte,error){t.Fatal("command issued for unapproved image");return nil,nil}}
  h:=HostReadOnlyAdapter{runner:runner,clock:time.Now}
  if _,e:=h.NormalizedCompose(context.Background(),reg,"web",ref);e==nil{t.Fatal("unapproved ref accepted",ref)}
 }
 before,_,_:=configs()
 before["services"].(map[string]any)["worker"].(map[string]any)["build"]="."
 runner:=&hostFixtureRunner{response:func(c hostReadCommand)([]byte,error){return raw(before),nil}}
 h:=HostReadOnlyAdapter{runner:runner,clock:time.Now}
 if _,e:=h.NormalizedCompose(context.Background(),reg,"web","");e==nil{
  t.Fatal("unsupported sibling build allowed")
 }
 if _,_,e:=h.FetchOCI(context.Background(),"invalid/unapproved-registry","sha256:"+strings.Repeat("a",64));!errors.Is(e,ErrPublicOCIUnavailable){
  t.Fatalf("unverified registry proof accepted: %v",e)
 }
}

func TestHostActualReadOnlyIdentityAndDrift(t *testing.T){
 reg:=fixtureRegistration()
 id:=strings.Repeat("a",64)
 image:="sha256:"+strings.Repeat("b",64)
 clock:=time.Date(2026,11,4,1,0,0,0,time.UTC)
 label:=map[string]string{"com.docker.compose.project":reg.ProjectName,"com.docker.compose.service":"web"}
 actual:=map[string]any{"Id":id,"Image":image,"Config":map[string]any{"Labels":label},
   "State":map[string]any{"Running":true}}
 r:=&hostFixtureRunner{response:func(c hostReadCommand)([]byte,error){
  if c.args[2]=="ps"{return []byte(id+"\n"),nil}
  if c.args[2]=="inspect"{return raw(actual),nil}
  t.Fatal("unexpected Docker operation",c.args)
  return nil,nil
 }}
 h:=HostReadOnlyAdapter{runner:r,clock:func()time.Time{return clock}}
 got,e:=h.InspectActual(context.Background(),reg,"web")
 if e!=nil||got.ContainerID!=id||got.ImageID!=image||!validDigest(got.SourceFingerprint)||
  !got.CheckedAt.Equal(clock){t.Fatalf("real Docker read-only baseline failed: %#v %v",got,e)}
 if len(r.calls)!=2||r.calls[0].args[2]!="ps"||r.calls[1].args[2]!="inspect"{
  t.Fatal("unexpected Docker operations",r.calls)
 }
 actual["State"].(map[string]any)["Running"]=false
 if _,e=h.InspectActual(context.Background(),reg,"web");e==nil{t.Fatal("stopped container qualified")}
 actual["State"].(map[string]any)["Running"]=true
 label["com.docker.compose.project"]="other"
 if _,e=h.InspectActual(context.Background(),reg,"web");e==nil{t.Fatal("wrong Compose label qualified")}
}

func TestHostActualRejectsMultipleContainersAndPoisonedOutput(t *testing.T){
 reg:=fixtureRegistration()
 id:=strings.Repeat("a",64)
 for _,out:=range []string{id+"\n"+id+"\n",id+"\nextra",strings.Repeat("Z",64),"",id+"  ",id+"\r\n"}{
  r:=&hostFixtureRunner{response:func(c hostReadCommand)([]byte,error){return []byte(out),nil}}
  h:=HostReadOnlyAdapter{runner:r,clock:time.Now}
  if _,e:=h.InspectActual(context.Background(),reg,"web");e==nil{t.Fatalf("poisoned docker ps output accepted: %q",out)}
 }
}
