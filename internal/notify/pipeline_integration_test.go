package notify

import(
 "context"
 "crypto/hmac"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/discovery"
 "github.com/docklantern/docklantern/internal/monitor"
 "github.com/docklantern/docklantern/internal/registry"
 "github.com/docklantern/docklantern/internal/store"
)

type scenarioResolver struct{count int}
func(r *scenarioResolver)Check(_ context.Context,image,platform string)(registry.Result,error){
 r.count++
 digest:="sha256:"+strings.Repeat("a",64)
 if r.count>1{digest="sha256:"+strings.Repeat("b",64)}
 return registry.Result{Image:image,Platform:platform,IndexDigest:digest,PlatformDigest:digest},nil
}
// Runs only on disposable GitHub Actions Docker hosts, never on production.
func TestDisposableComposePipeline(t *testing.T){
 if os.Getenv("DOCKLANTERN_INTEGRATION_TEST")!="1"{t.Skip("requires disposable Docker host")}
 ctx:=context.Background()
 found,err:=discovery.Scan(ctx,discovery.Shell{})
 if err!=nil{t.Fatal(err)}
 var containers []store.Container
 for _,c:=range found {
  if c.Project=="docklantern-ci-flow"&&c.Service=="web"{
   containers=append(containers,store.Container{ID:c.ID,Name:c.Name,Project:c.Project,Service:c.Service,Image:c.Image,ImageID:c.ImageID,State:c.State,Health:c.Health,RepoDigests:c.RepoDigests,ImageOS:c.ImageOS,ImageArchitecture:c.ImageArchitecture})
  }
 }
 if len(containers)!=1||containers[0].State!="running"{t.Fatalf("fixture Compose container not discovered: %+v",containers)}
 db,err:=store.Open(filepath.Join(t.TempDir(),"end-to-end.sqlite"));if err!=nil{t.Fatal(err)};defer db.Close()
 if err=db.Snapshot(ctx,containers);err!=nil{t.Fatal(err)}
 if err=db.Register(ctx,store.Registration{Name:"docklantern-ci-flow",ProjectDir:"/tmp/ci-fixture",Files:[]string{"/tmp/ci-fixture/compose.yaml"},Services:[]string{"web"}});err!=nil{t.Fatal(err)}
 p:=store.MonitorPolicy{Project:"docklantern-ci-flow",Service:"web",Image:"busybox:1.36",Platform:"linux/amd64",IntervalSeconds:60}
 if err=db.SetMonitorPolicy(ctx,p);err!=nil{t.Fatal(err)}
 resolver:=&scenarioResolver{}
 scheduler:=monitor.Scheduler{Storage:db,Resolver:resolver,Reporter:db,PollEvery:time.Second,CheckTimeout:10*time.Second,BatchSize:5}
 now:=time.Now().UTC()
 scheduler.Now=func()time.Time{return now}
 if count,err:=scheduler.RunOnce(ctx);err!=nil||count!=1{t.Fatalf("baseline %d %v",count,err)}
 now=now.Add(61*time.Second)
 if count,err:=scheduler.RunOnce(ctx);err!=nil||count!=1{t.Fatalf("digest change %d %v",count,err)}
 checks,err:=db.RegistryChecks(ctx);if err!=nil||len(checks)!=1||checks[0].Status!="changed"{t.Fatalf("registry %+v %v",checks,err)}
 events,err:=db.Events(ctx,10);if err!=nil||len(events)!=1||events[0].Kind!="remote_digest_changed"{t.Fatalf("events %+v %v",events,err)}
 secret:=strings.Repeat("s",40)
 delivered:=0
 tls:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  payload:=json.RawMessage{}
  if err:=json.NewDecoder(r.Body).Decode(&payload);err!=nil{t.Error(err)}
  mac:=hmac.New(sha256.New,[]byte(secret));_,_=mac.Write(payload)
  if r.Header.Get("X-DockLantern-Signature")!="sha256="+hex.EncodeToString(mac.Sum(nil)){t.Error("invalid webhook signature")}
  if r.Header.Get("X-DockLantern-Event-ID")!= "1"{t.Error("event identity mismatch")}
  delivered++;w.WriteHeader(http.StatusNoContent)
 }))
 defer tls.Close()
 sender,err:=New(tls.URL+"/events",secret);if err!=nil{t.Fatal(err)}
 sender.client=tls.Client() // Only the test fixture can dial loopback.
 worker:=Worker{Store:db,Sender:sender,Reporter:db,Limit:10,Interval:time.Second}
 if n,err:=worker.RunOnce(ctx);err!=nil||n!=1{t.Fatalf("webhook %d %v",n,err)}
 if n,err:=worker.RunOnce(ctx);err!=nil||n!=0{t.Fatalf("duplicate notification %d %v",n,err)}
 if delivered!=1{t.Fatalf("delivery count=%d",delivered)}
 events,_=db.Events(ctx,10)
 if events[0].DeliveryState!="delivered"{t.Fatalf("event not delivered: %+v",events[0])}
 if err=db.RecordWorkerRun(ctx,"registry_monitor",time.Now(),2,nil);err!=nil{t.Fatal(err)}
 if err=db.RecordWorkerRun(ctx,"webhook_notifier",time.Now(),1,nil);err!=nil{t.Fatal(err)}
 states,err:=db.WorkerStatuses(ctx);if err!=nil||len(states)!=2{t.Fatalf("worker status %+v %v",states,err)}
}
