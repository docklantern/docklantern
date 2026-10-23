// docklantern-inventory is a monitoring-only CLI/HTTP prototype.
// The HTTP interface requires credentials, and remains loopback-only.
package main

import (
 "context"
 "os/signal"
 "path/filepath"
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
 "github.com/docklantern/docklantern/internal/notify"
 "github.com/docklantern/docklantern/internal/registry"
 "github.com/docklantern/docklantern/internal/store"
 appRuntime "github.com/docklantern/docklantern/internal/runtime"
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
 inProcessMonitor:=flag.Bool("run-monitor",false,"run monitoring scheduler in the web server process")
 scanEvery:=flag.Duration("scan-every",0,"periodic Docker inventory scan; zero disables Docker access")
 runNotifier:=flag.Bool("run-notifier",false,"deliver opt-in monitoring event webhooks")
 backupPath:=flag.String("backup","","create a consistent SQLite backup at a new path, then exit")
 pruneEvents:=flag.Bool("prune-events",false,"explicitly remove delivered events older than configured retention")
 flag.Parse()
 scanDocker:=func(ctx context.Context)([]discovery.Container,error){
  if endpoint:=os.Getenv("DOCKLANTERN_DOCKER_PROXY_URL");endpoint!=""{
   proxy,err:=discovery.NewProxy(endpoint);if err!=nil{return nil,err}
   return proxy.Scan(ctx)
  }
  return discovery.Scan(ctx,discovery.Shell{})
 }
 ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer stop()
 if *mode=="discover"{x,e:=scanDocker(ctx);must(e);must(json.NewEncoder(os.Stdout).Encode(x));return}
 s,e:=store.Open(*dbPath);must(e);defer s.Close()
 if *backupPath!=""{must(s.Backup(ctx,*backupPath));fmt.Println("backup created");return}
 if *pruneEvents{n,e:=s.PruneDeliveredEvents(ctx,time.Now().UTC());must(e);fmt.Printf("pruned %d old delivered events\\n",n);return}
 setupNotifier:=func()notify.Worker{
  endpoint:=os.Getenv("DOCKLANTERN_WEBHOOK_URL")
  secretFile:=os.Getenv("DOCKLANTERN_WEBHOOK_HMAC_KEY_FILE")
  if endpoint==""||secretFile==""{log.Fatal("notifications require DOCKLANTERN_WEBHOOK_URL and DOCKLANTERN_WEBHOOK_HMAC_KEY_FILE")}
  secret,e:=os.ReadFile(filepath.Clean(secretFile));must(e)
  sender,e:=notify.New(endpoint,strings.TrimSpace(string(secret)));must(e)
  return notify.Worker{Store:s,Sender:sender,Reporter:s,Interval:10*time.Second,Limit:20}
 }

 switch *mode{
 case "register":
  b,e:=os.ReadFile(*registration);must(e);var p store.Registration;must(json.Unmarshal(b,&p))
  validate:=poc.Registration{Name:p.Name,ProjectDir:p.ProjectDir,Files:p.Files,EnvFiles:p.EnvFiles,Services:p.Services,Profiles:p.Profiles}
  if *roots==""{log.Fatal("registration requires explicit -roots")}
  must(validate.Validate(strings.Split(*roots,",")))
  p.Enabled=false // Monitoring registration never authorizes updates.
  must(s.Register(ctx,p));fmt.Println("registered for monitoring:",p.Name)
 case "scan":
  x,e:=scanDocker(ctx);must(e)
  items:=make([]store.Container,0,len(x))
  for _,c:=range x{items=append(items,store.Container{ID:c.ID,Name:c.Name,Project:c.Project,Service:c.Service,Image:c.Image,ImageID:c.ImageID,State:c.State,Health:c.Health,RepoDigests:c.RepoDigests,ImageOS:c.ImageOS,ImageArchitecture:c.ImageArchitecture,ImageVariant:c.ImageVariant})}
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
  scheduler:=monitor.Scheduler{Storage:s,Resolver:resolver,PollEvery:*pollEvery,CheckTimeout:*checkTimeout,BatchSize:20,Reporter:s}
  if *mode=="check-due"{n,e:=scheduler.RunOnce(ctx);if e!=nil{log.Printf("monitor checks failed: %v",e)};fmt.Printf("checked %d due policies\\n",n)}else{must(scheduler.Run(ctx))}
 case "events":
  records,e:=s.Events(ctx,100);must(e);must(json.NewEncoder(os.Stdout).Encode(records))
 case "notify":
  must(setupNotifier().Run(ctx))
 case "projects":
  x,e:=s.Projects(ctx);must(e);must(json.NewEncoder(os.Stdout).Encode(x))
 case "serve":
  // Non-loopback binding requires explicit HTTPS reverse-proxy configuration.
  proxyMode:=os.Getenv("DOCKLANTERN_ALLOW_PROXY")=="1"
  origin:=os.Getenv("DOCKLANTERN_PUBLIC_ORIGIN")
  local:=strings.HasPrefix(*addr,"127.0.0.1:")||strings.HasPrefix(*addr,"localhost:")
  if !local && (!proxyMode||origin==""||*addr!="0.0.0.0:8787") {
   log.Fatal("non-loopback requires DOCKLANTERN_ALLOW_PROXY=1, explicit HTTPS origin and -listen 0.0.0.0:8787")
  }
  if proxyMode!=(!local){log.Fatal("proxy mode and bind address must agree")}
  if *scanEvery!=0 && *scanEvery<time.Minute {log.Fatal("-scan-every must be zero or at least one minute")}
  log.Printf("inventory API on %s",*addr)
  username:=os.Getenv("DOCKLANTERN_ADMIN_USERNAME")
  password:=os.Getenv("DOCKLANTERN_ADMIN_PASSWORD")
  passwordFile:=os.Getenv("DOCKLANTERN_ADMIN_PASSWORD_FILE")
  if passwordFile!=""{
   if password!=""{log.Fatal("choose only one of DOCKLANTERN_ADMIN_PASSWORD and DOCKLANTERN_ADMIN_PASSWORD_FILE")}
   // File-based Docker secrets avoid putting passwords in Compose env values.
   secret,e:=os.ReadFile(filepath.Clean(passwordFile));must(e)
   password=strings.TrimSuffix(string(secret),"\n")
   password=strings.TrimSuffix(password,"\r")
  }
  guard,e:=auth.New(username,password);must(e)
  if proxyMode{must(guard.SetPublicOrigin(origin))}else if origin!=""{log.Fatal("public origin requires reverse-proxy mode")}

  var sourceRoots []string
  if *roots!=""{sourceRoots=strings.Split(*roots,",")}
  supervisor:=appRuntime.NewSupervisor()
  requirements:=[]appRuntime.Requirement{}
  if *runNotifier {
   worker:=setupNotifier()
   must(s.RecordWorkerStart(ctx,"webhook_notifier",time.Now().UTC()))
   requirements=append(requirements,appRuntime.Requirement{Name:"webhook_notifier",MaxAge:2*time.Minute})
   supervisor.Start(ctx,"webhook_notifier",worker.Run)
  }
  if *inProcessMonitor {
   resolver:=monitor.HostScopedResolver{RegistryHost:os.Getenv("DOCKLANTERN_REGISTRY_CREDENTIAL_HOST"),Username:os.Getenv("DOCKLANTERN_REGISTRY_USERNAME"),Token:os.Getenv("DOCKLANTERN_REGISTRY_TOKEN")}
   scheduler:=monitor.Scheduler{Storage:s,Resolver:resolver,PollEvery:*pollEvery,CheckTimeout:*checkTimeout,BatchSize:20,Reporter:s}
   must(s.RecordWorkerStart(ctx,"registry_monitor",time.Now().UTC()))
   maxAge:=3**pollEvery
   if maxAge<2*time.Minute{maxAge=2*time.Minute}
   // Long batches may legitimately take time; use the worst-case bounded
   // runtime as a conservative heartbeat freshness window.
   if batchWindow:=20**checkTimeout+*pollEvery;batchWindow>maxAge{maxAge=batchWindow}
   requirements=append(requirements,appRuntime.Requirement{Name:"registry_monitor",MaxAge:maxAge})
   supervisor.Start(ctx,"registry_monitor",scheduler.Run)
  }
  if *scanEvery>0 {
   must(s.RecordWorkerStart(ctx,"docker_discovery",time.Now().UTC()))
   requirements=append(requirements,appRuntime.Requirement{Name:"docker_discovery",MaxAge:3**scanEvery+40*time.Second})
   supervisor.Start(ctx,"docker_discovery",func(workerCtx context.Context)error{
    scan:=func()error{
     scanCtx,cancel:=context.WithTimeout(workerCtx,40*time.Second);defer cancel()
     items,err:=scanDocker(scanCtx)
     if err==nil{
      records:=make([]store.Container,0,len(items))
      for _,c:=range items{records=append(records,store.Container{ID:c.ID,Name:c.Name,Project:c.Project,Service:c.Service,Image:c.Image,ImageID:c.ImageID,State:c.State,Health:c.Health,RepoDigests:c.RepoDigests,ImageOS:c.ImageOS,ImageArchitecture:c.ImageArchitecture,ImageVariant:c.ImageVariant})}
      err=s.Snapshot(scanCtx,records)
     }
     if recordErr:=s.RecordWorkerRun(workerCtx,"docker_discovery",time.Now().UTC(),len(items),err);recordErr!=nil{return recordErr}
     // Docker outages are reflected in readiness; continue to recover.
     return nil
    }
    if err:=scan();err!=nil{return err}
    ticker:=time.NewTicker(*scanEvery);defer ticker.Stop()
    for{select{case <-workerCtx.Done():return nil;case <-ticker.C:if err:=scan();err!=nil{return err}}}
   })
  }
  readiness:=appRuntime.Checker{Reader:s,Required:requirements}
  server:=&http.Server{Addr:*addr,Handler:management.Handler(s,guard,sourceRoots,readiness),ReadHeaderTimeout:5*time.Second,ReadTimeout:10*time.Second,WriteTimeout:15*time.Second,IdleTimeout:60*time.Second}
  go func(){<-ctx.Done();shutdownCtx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel();_ = server.Shutdown(shutdownCtx)}()
  serveErrors:=make(chan error,1)
  go func(){err:=server.ListenAndServe();if err!=http.ErrServerClosed{serveErrors<-err}else{serveErrors<-nil}}()
  select{
  case err:=<-supervisor.Failures():
   log.Printf("critical background worker exited: %v",err)
   stop()
   shutdownCtx,cancel:=context.WithTimeout(context.Background(),10*time.Second);_ =server.Shutdown(shutdownCtx);cancel()
   os.Exit(1) // Let the container restart policy recover the whole service.
  case err:=<-serveErrors:
   if err!=nil{log.Fatal(err)}
  case <-ctx.Done():
   shutdownCtx,cancel:=context.WithTimeout(context.Background(),10*time.Second);_ =server.Shutdown(shutdownCtx);cancel()
  }
 default:log.Fatal("unknown mode")
 }
}
func must(err error){if err!=nil{log.Fatal(err)}}
