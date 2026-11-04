package executorpreflight

import (
 "errors"
 "os"
 "path/filepath"
 "testing"
 "syscall"

 "github.com/docklantern/docklantern/internal/executorregistration"
)

func sourceFixture(t *testing.T)(executorregistration.Registration,string,string) {
 t.Helper()
 if os.Geteuid()!=0 || os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1" {
  t.Skip("isolated root-only CI fixture")
 }
 root,e:=os.MkdirTemp("/root","docklantern-preflight-sources-")
 if e!=nil{t.Fatal(e)}
 t.Cleanup(func(){_ = os.RemoveAll(root)})
 if e=os.Chmod(root,0o700);e!=nil{t.Fatal(e)}
 dir:=filepath.Join(root,"project")
 if e=os.Mkdir(dir,0o700);e!=nil{t.Fatal(e)}
 compose:=filepath.Join(dir,"compose.yaml")
 env:=filepath.Join(dir,"service.env")
 if e=os.WriteFile(compose,[]byte("services:\n  web:\n    image: docker.io/example/hello-go:stable\n"),0o600);e!=nil{t.Fatal(e)}
 if e=os.WriteFile(env,[]byte("SAMPLE=value\n"),0o600);e!=nil{t.Fatal(e)}
 uid:=uint32(0)
 reg:=executorregistration.Registration{
  SchemaVersion:executorregistration.SchemaVersion,
  RegistrationID:"preflight-registration",ProjectName:"demo-project",
  ProjectDirectory:dir,ComposeFiles:[]string{compose},EnvFiles:[]string{env},
  TrustedSourceUID:&uid,FeaturePolicy:executorregistration.FeaturePolicyPrebuilt,
  Services:[]executorregistration.Service{{
   Name:"web",DeploymentEnabled:true,ApprovedRepository:"docker.io/example/hello-go",
   Platform:executorregistration.Platform{OS:"linux",Architecture:"amd64"},
  }},
 }
 if e=reg.Validate();e!=nil{t.Fatal(e)}
 return reg,compose,env
}

func TestTrustedSourceSnapshotAndRecheckRootFixture(t *testing.T){
 reg,compose,env:=sourceFixture(t)
 first,e:=CaptureTrustedSources(reg)
 if e!=nil||first.FileCount!=2||!validDigest(first.SHA256){t.Fatalf("first capture: %#v %v",first,e)}
 if e=RecheckTrustedSources(reg,first);e!=nil{t.Fatal(e)}
 if e=os.WriteFile(env,[]byte("SAMPLE=changed\n"),0o600);e!=nil{t.Fatal(e)}
 if e=RecheckTrustedSources(reg,first);!errors.Is(e,ErrSourceChanged){t.Fatal("changed env accepted",e)}
 if e=os.WriteFile(env,[]byte("SAMPLE=value\n"),0o600);e!=nil{t.Fatal(e)}
 if e=RecheckTrustedSources(reg,first);e!=nil{t.Fatal("exact bytes rejected",e)}
 // Restore matching content and reorder registered sources: the hash is not
 // a commutative set and must bind exact Compose/env file precedence.
 reg.ComposeFiles=[]string{env}
 reg.EnvFiles=[]string{compose}
 swapped,e:=CaptureTrustedSources(reg)
 if e!=nil||swapped.SHA256==first.SHA256{t.Fatal("registered file order not bound",e)}
}

func TestTrustedSourceSnapshotRejectsUnsafeObjectsRootFixture(t *testing.T){
 cases:=[]struct{name string;attack func(*testing.T,executorregistration.Registration,string,string)}{
  {"symlink",func(t *testing.T,r executorregistration.Registration,c,e string){
    if er:=os.Remove(c);er!=nil{t.Fatal(er)}
    if er:=os.Symlink(e,c);er!=nil{t.Fatal(er)}
  }},
  {"group_writable",func(t *testing.T,r executorregistration.Registration,c,e string){
    if er:=os.Chmod(c,0o660);er!=nil{t.Fatal(er)}
  }},
  {"hardlink",func(t *testing.T,r executorregistration.Registration,c,e string){
    if er:=os.Link(c,filepath.Join(r.ProjectDirectory,"alias"));er!=nil{t.Fatal(er)}
  }},
  {"writable_parent",func(t *testing.T,r executorregistration.Registration,c,e string){
    if er:=os.Chmod(r.ProjectDirectory,0o770);er!=nil{t.Fatal(er)}
  }},
  {"fifo",func(t *testing.T,r executorregistration.Registration,c,e string){
    if er:=os.Remove(c);er!=nil{t.Fatal(er)}
    if er:=syscall.Mkfifo(c,0o600);er!=nil{t.Fatal(er)}
  }},
  {"missing",func(t *testing.T,r executorregistration.Registration,c,e string){
    if er:=os.Remove(c);er!=nil{t.Fatal(er)}
  }},
  {"oversize",func(t *testing.T,r executorregistration.Registration,c,e string){
    if er:=os.WriteFile(c,make([]byte,maxSourceBytes+1),0o600);er!=nil{t.Fatal(er)}
  }},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  reg,compose,env:=sourceFixture(t)
  tc.attack(t,reg,compose,env)
  if _,e:=CaptureTrustedSources(reg);!errors.Is(e,ErrSourceChanged){
   t.Fatalf("unsafe registered source accepted: %v",e)
  }
 })}
}

func TestTrustedSourceSnapshotRejectsInvalidRegistrationRootFixture(t *testing.T){
 reg,_,_:=sourceFixture(t)
 reg.ComposeFiles=[]string{"/tmp/untrusted.yaml"}
 if _,e:=CaptureTrustedSources(reg);!errors.Is(e,ErrSourceChanged){t.Fatal("outside project accepted",e)}
}
