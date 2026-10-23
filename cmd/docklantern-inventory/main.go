// docklantern-inventory is a monitoring-only CLI/HTTP prototype.
// The HTTP interface requires credentials, and remains loopback-only.
package main

import (
 "context"
 "os/signal"
 "syscall"
 "runtime"
 "encoding/json"
 "flag"
 "fmt"
 "log"
 "net/http"
 "os"
 "strings"
 "time"
 "github.com/docklantern/docklantern/internal/auth"
 "github.com/docklantern/docklantern/internal/management"
 "github.com/docklantern/docklantern/internal/discovery"
 "github.com/docklantern/docklantern/internal/poc"
 "github.com/docklantern/docklantern/internal/monitor"
 "github.com/docklantern/docklantern/internal/registry"
 "github.com/docklantern/docklantern/internal/store"
)
func main(){
 mode:=flag.String("mode","serve","serve, discover, scan, register, projects, check-image, set-policy, policies, check-due, monitor")
 dbPath:=flag.String("db","/data/docklantern.db","SQLite file")
 addr:=flag.String("listen","127.0.0.1:8787","local listener only")
 registration:=flag.String("registration","","JSON registration file")
 image:=flag.String("image","","tagged OCI image for manual registry check")
 platform:=flag.String("platform",runtime.GOOS+"/"+runtime.GOARCH,"target platform")
 roots:=flag.String("roots","","comma-separated allowlisted Compose source roots")
 policyFile:=flag.String("policy-file","","monitoring-only policy JSON")
 checkTimeout:=flag.Duration("check-timeout",20*time.Second,"timeout per manifest check")
 pollEvery:=flag.Duration("poll-every",10*time.Second,"due job scan frequency")
 flag.Parse()
 ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer stop()
 if *mode=="discover"{x,e:=discovery.Scan(ctx,discovery.Shell{});must(e);must(json.NewEncoder(os.Stdout).Encode(x));return}
 s,e:=store.Open(*dbPath);must(e);defer s.Close()
 switch *mode{
 case "register":
  b,e:=os.ReadFile(*registration);must(e);var p store.Registration;must(json.Unmarshal(b,&p))
  validate:=poc.Registration{Name:p.Name,ProjectDir:p.ProjectDir,Files:p.Files,EnvFiles:p.EnvFiles,Services:p.Services,Profiles:p.Profiles}
  if *roots==""{log.Fatal("registration requires explicit -roots")}
  must(validate.Validate(strings.Split(*roots,",")))
  p.Enabled=false // Monitoring registration never authorizes updates.
  must(s.Register(ctx,p));fmt.Println("registered for monitoring:",p.Name)
 case "scan":
  x,e:=discovery.Scan(ctx,discovery.Shell{});must(e)
  items:=make([]store.Container,0,len(x))
  for _,c:=range x{items=append(items,store.Container{ID:c.ID,Name:c.Name,Project:c.Project,Service:c.Service,Image:c.Image,ImageID:c.ImageID,State:c.State,Health:c.Health})}
  must(s.Snapshot(ctx,items));fmt.Printf("recorded %d containers\n",len(items))
 case "check-image":
  if *image==""{log.Fatal("-image is required for check-image")}
  user:=os.Getenv("DOCKLANTERN_REGISTRY_USERNAME")
  token:=os.Getenv("DOCKLANTERN_REGISTRY_TOKEN")
  if token!=""&&user==""{log.Fatal("registry username required with token")}
  resolution,e:=(registry.Client{Credentials:registry.Credentials{Username:user,Password:token}}).Check(ctx,*image,*platform);must(e)
  check,e:=s.RecordRegistryCheck(ctx,resolution.Image,resolution.Platform,resolution.IndexDigest,resolution.PlatformDigest);must(e)
  must(json.NewEncoder(os.Stdout).Encode(check))
 case "set-policy":
  if *policyFile==""{log.Fatal("set-policy requires -policy-file")}
  b,e:=os.ReadFile(*policyFile);must(e)
  var p store.MonitorPolicy;must(json.Unmarshal(b,&p));must(s.SetMonitorPolicy(ctx,p))
  fmt.Printf("monitor-only policy saved: %s/%s\\n",p.Project,p.Service)
 case "policies":
  p,e:=s.MonitorPolicies(ctx);must(e);must(json.NewEncoder(os.Stdout).Encode(p))
 case "check-due","monitor":
  resolver:=monitor.HostScopedResolver{RegistryHost:os.Getenv("DOCKLANTERN_REGISTRY_CREDENTIAL_HOST"),Username:os.Getenv("DOCKLANTERN_REGISTRY_USERNAME"),Token:os.Getenv("DOCKLANTERN_REGISTRY_TOKEN")}
  scheduler:=monitor.Scheduler{Storage:s,Resolver:resolver,PollEvery:*pollEvery,CheckTimeout:*checkTimeout,BatchSize:20}
  if *mode=="check-due"{n,e:=scheduler.RunOnce(ctx);if e!=nil{log.Printf("monitor checks failed: %v",e)};fmt.Printf("checked %d due policies\\n",n)}else{must(scheduler.Run(ctx))}
 case "projects":
  x,e:=s.Projects(ctx);must(e);must(json.NewEncoder(os.Stdout).Encode(x))
 case "serve":
  // Authentication is required and the initial server remains loopback-only.
  if !strings.HasPrefix(*addr,"127.0.0.1:")&&!strings.HasPrefix(*addr,"localhost:"){log.Fatal("POC HTTP API must bind loopback")}
  log.Printf("inventory API on %s",*addr)
  username:=os.Getenv("DOCKLANTERN_ADMIN_USERNAME")
  password:=os.Getenv("DOCKLANTERN_ADMIN_PASSWORD")
  guard,e:=auth.New(username,password);must(e)
  var sourceRoots []string
  if *roots!=""{sourceRoots=strings.Split(*roots,",")}
  server:=&http.Server{Addr:*addr,Handler:management.Handler(s,guard,sourceRoots),ReadHeaderTimeout:5*time.Second,ReadTimeout:10*time.Second,WriteTimeout:15*time.Second,IdleTimeout:60*time.Second}
  log.Fatal(server.ListenAndServe())
 default:log.Fatal("unknown mode")
 }
}
func must(err error){if err!=nil{log.Fatal(err)}}
