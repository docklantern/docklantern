package management

import (
 "context"
 "net/http"
 "net/http/httptest"
 "net/url"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/store"
)

func testMonitoredContainer(project,service,image string,enabled bool)store.Container{
 label:="true"
 if !enabled{label="false"}
 return store.Container{ID:project+"-"+service+"-1",Project:project,Service:service,
  Image:image,State:"running",ImageOS:"linux",ImageArchitecture:"amd64",
  MonitorLabels:map[string]string{"com.docklantern.monitor":label}}
}

func TestBeta5ServiceWatchControlsAndCheckNow(t *testing.T){
 h,db,_,guard:=setup(t)
 ctx:=context.Background()
 c:=testMonitoredContainer("site","web","nginx:1.2.3",true)
 if err:=db.Snapshot(ctx,[]store.Container{c});err!=nil{t.Fatal(err)}
 page:=request(h,"GET","/services/site/web","",true)
 if page.Code!=200{t.Fatal(page.Code)}
 for _,term:=range []string{"Docker-observed container image","nginx:1.2.3",
  "Registry watch image","Watch target source","Watched registry tag",
  "Save watched tag","Check Now","Last successful registry observation",
  "Consecutive check failures","Last manual check request"}{
  if !strings.Contains(page.Body.String(),term){t.Errorf("missing service UI %q",term)}
 }
 path:="/settings/monitoring/watch"
 f:=url.Values{"project":{"site"},"service":{"web"},"action":{"set"},
   "image":{"nginx:latest"},"csrf_token":{guard.Token()}}
 if x:=request(h,"POST",path,f.Encode(),false);x.Code!=401{t.Fatalf("no auth: %d",x.Code)}
 f.Del("csrf_token")
 if x:=request(h,"POST",path,f.Encode(),true);x.Code!=403{t.Fatalf("no CSRF: %d",x.Code)}
 f.Set("csrf_token",guard.Token())
 forged:=httptest.NewRequest("POST","http://127.0.0.1:8787"+path,strings.NewReader(f.Encode()))
 forged.SetBasicAuth("admin",strings.Repeat("very-secure-demo-password-",2))
 forged.Header.Set("Origin","https://evil.example")
 forged.Header.Set("Content-Type","application/x-www-form-urlencoded")
 result:=httptest.NewRecorder();h.ServeHTTP(result,forged)
 if result.Code!=403{t.Fatalf("cross-origin watch accepted: %d",result.Code)}
 f.Set("image","redis:latest")
 if x:=request(h,"POST",path,f.Encode(),true);x.Code!=400{t.Fatalf("cross-repo watch accepted: %d",x.Code)}
 f.Set("image","nginx:latest")
 x:=request(h,"POST",path,f.Encode(),true)
 if x.Code!=303||x.Header().Get("Location")!="/services/site/web?saved=watch#monitoring-controls"{
  t.Fatalf("watch update redirect: %d %s",x.Code,x.Header().Get("Location"))
 }
 policies,err:=db.MonitorPolicies(ctx)
 if err!=nil||len(policies)!=1||policies[0].Image!="registry-1.docker.io/library/nginx:latest"{
  t.Fatalf("watch target not changed: %+v %v",policies,err)
 }
 page=request(h,"GET","/services/site/web?saved=watch","",true)
 for _,term:=range []string{"GUI watch override","Registry watch target saved",
   "registry-1.docker.io/library/nginx:latest","nginx:1.2.3","Reset watch to Docker-observed image"}{
  if !strings.Contains(page.Body.String(),term){t.Errorf("missing watch provenance %q",term)}
 }
 check:=url.Values{"project":{"site"},"service":{"web"},"csrf_token":{guard.Token()}}
 x=request(h,"POST","/settings/monitoring/check-now/service",check.Encode(),true)
 if x.Code!=303{t.Fatalf("check now: %d",x.Code)}
 if !strings.Contains(x.Header().Get("Location"),"check=already_due"){
  t.Fatalf("not correctly explaining already due: %s",x.Header().Get("Location"))
 }
 page=request(h,"GET","/services/site/web?check=already_due","",true)
 if !strings.Contains(page.Body.String(),"already due")||!strings.Contains(page.Body.String(),`role="status"`){
  t.Fatal("missing accessible Check Now notice")
 }
 f.Set("action","reset")
 f.Del("image")
 x=request(h,"POST",path,f.Encode(),true)
 if x.Code!=303{t.Fatalf("watch reset: %d",x.Code)}
 policies,_=db.MonitorPolicies(ctx)
 if policies[0].Image!="registry-1.docker.io/library/nginx:1.2.3"{
  t.Fatalf("reset did not restore Docker image: %+v",policies[0])
 }
 if x=request(h,"GET","/services/site/web?saved=forged","",true);x.Code!=400{
  t.Fatalf("invalid status accepted: %d",x.Code)
 }
 if x=request(h,"GET","/services/site/web?check=forged","",true);x.Code!=400{
  t.Fatalf("forged check notice accepted: %d",x.Code)
 }
}

func TestBeta5ImageCheckEligibilityAndSharedScope(t *testing.T){
 h,db,_,guard:=setup(t)
 ctx:=context.Background()
 a:=testMonitoredContainer("a","web","nginx:latest",true)
 b:=testMonitoredContainer("b","web","nginx:latest",true)
 c:=testMonitoredContainer("c","web","nginx:latest",false)
 if err:=db.Snapshot(ctx,[]store.Container{a,b,c});err!=nil{t.Fatal(err)}
 image:=presentation.ImageIdentity{Reference:"registry-1.docker.io/library/nginx:latest",Platform:"linux/amd64"}
 id:=presentation.ImageID(image)
 page:=request(h,"GET","/images/"+id,"",true)
 if page.Code!=200{t.Fatalf("image detail: %d",page.Code)}
 for _,term:=range []string{"2 eligible watcher(s)","Check Now",
   `action="/settings/monitoring/check-now/image"`,
   `name="image_id" value="`+id+`"`,`name="csrf_token"`}{
  if !strings.Contains(page.Body.String(),term){t.Errorf("missing image control %q",term)}
 }
 if x:=request(h,"GET","/images/"+id+"?check=invalid","",true);x.Code!=400{
  t.Fatalf("accepted forged image status: %d",x.Code)
 }
 f:=url.Values{"image_id":{id},"csrf_token":{guard.Token()}}
 x:=request(h,"POST","/settings/monitoring/check-now/image",f.Encode(),true)
 if x.Code!=303 || !strings.Contains(x.Header().Get("Location"),"check=already_due"){
  t.Fatalf("image check: %d %s",x.Code,x.Header().Get("Location"))
 }
 if err:=db.SetServicePaused(ctx,"a","web",true);err!=nil{t.Fatal(err)}
 if err:=db.SetServicePaused(ctx,"b","web",true);err!=nil{t.Fatal(err)}
 page=request(h,"GET","/images/"+id,"",true)
 if page.Code!=200||strings.Contains(page.Body.String(),`action="/settings/monitoring/check-now/image"`)||
  !strings.Contains(page.Body.String(),"No eligible watches"){
  t.Fatal("paused image still displayed an active check action")
 }
 if x:=request(h,"POST","/settings/monitoring/check-now/image",f.Encode(),true);x.Code!=409{
  t.Fatalf("paused group accepted manual check: %d",x.Code)
 }
}

func TestBeta5WatchRecoveryAndLabelVetoInDetails(t *testing.T){
 h,db,_,guard:=setup(t)
 ctx:=context.Background()
 enabled:=testMonitoredContainer("site","web","nginx:stable",true)
 disabled:=testMonitoredContainer("blocked","web","redis:latest",false)
 if err:=db.Snapshot(ctx,[]store.Container{enabled,disabled});err!=nil{t.Fatal(err)}
 target:="nginx:latest"
 if err:=db.SetServiceWatchOverride(ctx,"site","web",&target);err!=nil{t.Fatal(err)}
 enabled.Image="redis:latest"
 if err:=db.Snapshot(ctx,[]store.Container{enabled,disabled});err!=nil{t.Fatal(err)}
 page:=request(h,"GET","/services/site/web","",true)
 if page.Code!=200||!strings.Contains(page.Body.String(),"Reset watch to Docker-observed image")||
   strings.Contains(page.Body.String(),"Save watched tag") ||
   strings.Contains(page.Body.String(),`action="/settings/monitoring/check-now/service"`){
  t.Fatal("repository-conflict UI exposed unsafe actions")
 }
 f:=url.Values{"project":{"site"},"service":{"web"},"action":{"reset"},"csrf_token":{guard.Token()}}
 if x:=request(h,"POST","/settings/monitoring/watch",f.Encode(),true);x.Code!=303{
  t.Fatalf("safe recovery reset unavailable: %d",x.Code)
 }
 if p,err:=db.MonitorPolicies(ctx);err!=nil||len(p)!=1||p[0].Image!="registry-1.docker.io/library/redis:latest"{
  t.Fatalf("recovery did not inherit new Docker reference %+v %v",p,err)
 }
 blocked:=request(h,"GET","/services/blocked/web","",true)
 if blocked.Code!=200 || strings.Contains(blocked.Body.String(),`action="/settings/monitoring/watch"`)||
    strings.Contains(blocked.Body.String(),`action="/settings/monitoring/check-now/service"`){
  t.Fatal("explicit label opt-out UI exposed mutation actions")
 }
 f.Set("project","blocked");f.Set("action","set");f.Set("image","redis:stable")
 if x:=request(h,"POST","/settings/monitoring/watch",f.Encode(),true);x.Code!=400{
  t.Fatalf("forged disabled-label watch accepted: %d",x.Code)
 }
 if x:=request(h,"GET","/services/site/web","",false);x.Code!=401{
  t.Fatalf("unauthenticated service details available: %d",x.Code)
 }
 // UI never contacts a registry. A request changes only the backend watch;
 // background monitor must still process its own interval/backoff schedule.
 if len(request(h,"GET","/services/site/web","",true).Body.String())==0{t.Fatal("empty details")}
}
