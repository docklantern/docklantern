package executorapply

import (
 "context"
 "errors"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/executorhost"
 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executorpreflight"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

type nativeFixtureRunner struct{
 calls []nativeCommand
 version string
 fail bool
}
func(f *nativeFixtureRunner)run(_ context.Context,c nativeCommand)([]byte,error){
 if !approvedNativeArgs(c.args){return nil,errors.New("command outside allowlist")}
 f.calls=append(f.calls,c)
 if f.fail{return nil,errors.New("subprocess failed")}
 switch c.args[2]{
 case "version":return []byte(f.version),nil
 case "pull":return []byte("immutable pulled"),nil
 case "image":return []byte("sha256:"+strings.Repeat("9",64)+"\n"),nil
 }
 return nil,errors.New("unexpected fixture command")
}
func nativeRegistrationFixture()(executorregistration.Registration,executorpreflight.Qualification,string){
 uid:=uint32(1000)
 repository:="docker.io/example/hello-go"
 reg:=executorregistration.Registration{
  SchemaVersion:executorregistration.SchemaVersion,RegistrationID:"registered-demo",
  ProjectName:"hello-go",ProjectDirectory:"/srv/hello-go",
  ComposeFiles:[]string{"/srv/hello-go/compose.yaml"},
  TrustedSourceUID:&uid,FeaturePolicy:executorregistration.FeaturePolicyPrebuilt,
  Services:[]executorregistration.Service{{
   Name:"web",DeploymentEnabled:true,ApprovedRepository:repository,
   Platform:executorregistration.Platform{OS:"linux",Architecture:"amd64"},
   AllowRunningOnly:true,
  }},
 }
 q:=executorpreflight.Qualification{
  ProposalID:"proposal-demo",RegistrationID:"registered-demo",
  ProposalSHA256:"sha256:"+strings.Repeat("a",64),
  RegistrationSHA256:"sha256:"+strings.Repeat("b",64),
  SourceSHA256:"sha256:"+strings.Repeat("c",64),
  CandidateDigest:"sha256:"+strings.Repeat("d",64),
  ContainerID:strings.Repeat("e",64),
  ImageID:"sha256:"+strings.Repeat("f",64),
  Service:"web",ReadOnly:true,
 }
 return reg,q,repository
}
func TestNativeCommandAllowlistRefusesProjectWideOperations(t *testing.T){
 sock:=executorhost.LocalSocket
 prefix:=[]string{"--host",sock}
 denied:=[][]string{
  {"--host","tcp://evil:2375","pull","docker.io/example/app@sha256:"+strings.Repeat("a",64)},
  append(append([]string(nil),prefix...),"compose","down","-v"),
  append(append([]string(nil),prefix...),"compose","up","-d"),
  append(append([]string(nil),prefix...),"run","--privileged","alpine"),
  append(append([]string(nil),prefix...),"image","rm","hello"),
  append(append([]string(nil),prefix...),"pull","docker.io/example/app:latest"),
  append(append([]string(nil),prefix...),"inspect","--format","{{json .}}","--","../../escape"),
 }
 for _,args:=range denied{
  if approvedNativeArgs(args){t.Fatalf("unsafe Docker command accepted: %#v",args)}
 }
 valid:=[]string{"--host",sock,"compose","--project-directory","/srv/hello-go",
  "-p","hello-go","-f",executorpreflight.HostSourceSnapshotsDirectory+"/source-123/input-0.yaml",
  "-f","-","up","--no-deps","--no-build","--pull","never","--force-recreate","-d","web"}
 if !approvedNativeArgs(valid){t.Fatal("narrow selected-service Compose up denied")}
 valid[len(valid)-1]=""
 if approvedNativeArgs(valid){t.Fatal("empty target service accepted")}
}
func TestNativeEffectVersionAndImmutablePullContract(t *testing.T){
 reg,q,repository:=nativeRegistrationFixture()
 for _,tc:=range []struct{version string; allowed bool}{
  {`{"Version":"29.8.1","ApiVersion":"1.56","Os":"linux","Arch":"amd64"}`,true},
  {`{"Version":"29.8.0","ApiVersion":"1.56","Os":"linux","Arch":"amd64"}`,false},
  {`{"Version":"29.8.1","ApiVersion":"1.55","Os":"linux","Arch":"amd64"}`,false},
  {`{"Version":"29.8.1","ApiVersion":"1.56","Os":"linux","Arch":"arm64"}`,false},
  {"not json",false},
 }{
  runner:=&nativeFixtureRunner{version:tc.version}
  native,err:=newHostEffects(reg,q,repository,runner)
  if err!=nil{t.Fatal(err)}
  err=native.PullImmutable(context.Background(),sampleOperationForNative(q))
  if (err==nil)!=tc.allowed{t.Fatalf("runtime policy inconsistent: %q %v",tc.version,err)}
  if tc.allowed{
   if native.pulledImageID!="sha256:"+strings.Repeat("9",64)||
    len(runner.calls)!=3||runner.calls[1].args[3]!="docker.io/example/hello-go@"+q.CandidateDigest{
    t.Fatalf("wrong immutable pull ordering: %#v",runner.calls)
   }
  }else if len(runner.calls)!=1{
   t.Fatalf("unqualified runtime performed write: %#v",runner.calls)
  }
 }
}
func sampleOperationForNative(q executorpreflight.Qualification)executorjournal.Operation{
 return executorjournal.Operation{ID:"native-test",ProjectID:"hello-go",ServiceID:q.Service,
  ProposalID:q.ProposalID,CandidateDigest:q.CandidateDigest}
}
