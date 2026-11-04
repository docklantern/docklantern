package executorpreflight

import (
 "context"
 "os"
 "os/exec"
 "reflect"
 "path/filepath"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/executorregistration"
)

// This fixture exercises the *actual* installed Docker Compose plugin's
// read-only config command. It never starts or modifies containers. Explicit
// root + opt-in prevent the test from touching local operator projects.
func TestRealComposeConfigReadOnlyRootFixture(t *testing.T) {
 if os.Geteuid()!=0 || os.Getenv("DOCKLANTERN_PREFLIGHT_COMPOSE_FIXTURE")!="1" {
  t.Skip("root-only disposable Compose configuration fixture")
 }
 if err:=exec.Command("/usr/bin/docker","compose","version").Run();err!=nil{
  t.Fatalf("Compose plugin unavailable in guarded CI: %v",err)
 }
 root,err:=os.MkdirTemp("/root","docklantern-compose-readonly-")
 if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){_ = os.RemoveAll(root)})
 if err=os.Chmod(root,0o700);err!=nil{t.Fatal(err)}
 project:=filepath.Join(root,"hello-project")
 if err=os.Mkdir(project,0o700);err!=nil{t.Fatal(err)}
 compose:=filepath.Join(project,"compose.yaml")
 env:=filepath.Join(project,"compose.env")
 const input=`services:
  web:
    image: docker.io/example/hello-go:stable
    pull_policy: always
    expose:
      - "8080"
    labels:
      traefik.enable: "true"
      traefik.http.routers.hello.rule: "Host(`+"\x60"+`hello.example.invalid`+"\x60"+`)"
    environment:
      SAMPLE: "${SAMPLE_VALUE}"
    networks:
      - web
  sibling:
    image: docker.io/example/sibling:stable
    networks:
      - web
networks:
  web:
    external: true
`
 if err=os.WriteFile(compose,[]byte(input),0o600);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(env,[]byte("SAMPLE_VALUE=stable-value\n"),0o600);err!=nil{t.Fatal(err)}
 uid:=uint32(0)
 reg:=executorregistration.Registration{
  SchemaVersion:1,RegistrationID:"hello-registration",ProjectName:"hello-project",
  ProjectDirectory:project,ComposeFiles:[]string{compose},EnvFiles:[]string{env},
  TrustedSourceUID:&uid,FeaturePolicy:executorregistration.FeaturePolicyPrebuilt,
  Services:[]executorregistration.Service{{Name:"web",DeploymentEnabled:true,
   ApprovedRepository:"docker.io/example/hello-go",
   Platform:executorregistration.Platform{OS:"linux",Architecture:"amd64"}}},
 }
 if err=reg.Validate();err!=nil{t.Fatal(err)}
 start,err:=CaptureTrustedSources(reg)
 if err!=nil{t.Fatalf("trusted source capture failed: %v",err)}
 h,err:=NewHostReadOnlyAdapter()
 if err!=nil{t.Fatalf("local trusted docker unavailable: %v",err)}
 ctx:=context.Background()
 before,err:=h.NormalizedCompose(ctx,reg,"web","")
 if err!=nil{t.Fatalf("real Compose baseline config failed (source/env/profiles): %v",err)}
 digest:="sha256:"+strings.Repeat("a",64)
 after,err:=h.NormalizedCompose(ctx,reg,"web","docker.io/example/hello-go@"+digest)
 if err!=nil{t.Fatalf("real Compose stdin override failed: %v",err)}
 if err=CheckImageOnly(before,after,reg,"web","docker.io/example/hello-go",digest);err!=nil{
  t.Fatalf("real Compose config differs beyond selected image: %v",err)
 }
 if err=RecheckTrustedSources(reg,start);err!=nil{t.Fatalf("no-change source drift: %v",err)}
 // Consume immutable private Compose/env copies while explicitly retaining
 // the original approved project directory and image-only semantics.
 handoff,err:=stageTrustedSourcesAt(reg,"/root")
 if err!=nil{t.Fatalf("root-private handoff failed: %v",err)}
 defer handoff.Close()
 stagedBefore,err:=h.NormalizedComposeFromHandoff(ctx,reg,handoff,"web","")
 if err!=nil{t.Fatalf("real Compose staged baseline failed: %v",err)}
 stagedAfter,err:=h.NormalizedComposeFromHandoff(ctx,reg,handoff,"web","docker.io/example/hello-go@"+digest)
 if err!=nil{t.Fatalf("real Compose staged candidate failed: %v",err)}
 if err=CheckImageOnly(stagedBefore,stagedAfter,reg,"web","docker.io/example/hello-go",digest);err!=nil{
  t.Fatalf("staged Compose changed non-image configuration: %v",err)
 }
 originalConfig,_:=parseNormalized(before)
 stagedConfig,_:=parseNormalized(stagedBefore)
 if !reflect.DeepEqual(originalConfig,stagedConfig){
  t.Fatal("staged Compose baseline differs from original normalization")
 }
 // Real Compose source changes are detected, not treated as permission to
 // execute a stale plan. Never print raw configuration or environment.
 if err=os.WriteFile(env,[]byte("SAMPLE_VALUE=changed-value\n"),0o600);err!=nil{t.Fatal(err)}
 if err=RecheckTrustedSources(reg,start);err==nil{
  t.Fatal("changed Compose interpolation source accepted")
 }
 if _,err=h.NormalizedComposeFromHandoff(ctx,reg,handoff,"web","");err==nil{
  t.Fatal("staged Compose accepted a changed original source")
 }
 // A service env_file is an implicit Compose input that would not be
 // pinned by the registered top-level source snapshot. --no-env-resolution
 // must retain this field so that the strict feature policy rejects it.
 implicit:=filepath.Join(project,"unregistered.env")
 if err=os.WriteFile(implicit,[]byte("SECRET=unregistered\\n"),0o600);err!=nil{t.Fatal(err)}
 edited:=strings.Replace(input,"    pull_policy: always",
  "    pull_policy: always\\n    env_file:\\n      - ./unregistered.env",1)
 if err=os.WriteFile(compose,[]byte(edited),0o600);err!=nil{t.Fatal(err)}
 if _,err=h.NormalizedCompose(ctx,reg,"web","");err==nil{
  t.Fatal("Compose silently resolved an unregistered service env_file")
 }
}
