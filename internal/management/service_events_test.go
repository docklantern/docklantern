package management

import (
 "context"
 "net/http"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/store"
)

func TestEventHistoryExplainsImageScopeAndWebhookState(t *testing.T){
 h,db,_,_:=setup(t)
 ctx:=context.Background()
 containers:=[]store.Container{}
 for _,project:=range []string{"hello-js","hello-py"}{
  containers=append(containers,store.Container{
   ID:project+"-id",Name:project+"-web-1",Project:project,Service:"web",
   Image:"ghcr.io/example/shared:latest",ImageID:"sha256:config",
   ImageOS:"linux",ImageArchitecture:"amd64",State:"running",
   MonitorLabels:map[string]string{"com.docklantern.monitor":"true"},
  })
 }
 if err:=db.Snapshot(ctx,containers);err!=nil{t.Fatal(err)}
 policies,err:=db.MonitorPolicies(ctx)
 if err!=nil||len(policies)!=2{t.Fatalf("two watchers not discovered: %+v %v",policies,err)}
 image,platform:=policies[0].Image,policies[0].Platform
 for i,digest:=range []string{"sha256:first","sha256:changed","sha256:changed"}{
  check,err:=db.RecordRegistryCheck(ctx,image,platform,"sha256:index",digest)
  if err!=nil{t.Fatal(err)}
  if check.Status!=[]string{"first_seen","changed","unchanged"}[i]{t.Fatalf("step %d: %+v",i,check)}
 }
 events,err:=db.Events(ctx,20)
 if err!=nil||len(events)!=1{t.Fatalf("registry should emit one image event %+v %v",events,err)}
 if events[0].Project!=""||events[0].Service!=""{t.Fatalf("global event was given incorrect owner %+v",events[0])}
 response:=request(h,http.MethodGet,"/events?search=ghcr.io%2Fexample%2Fshared%3Alatest","",true)
 if response.Code!=http.StatusOK{t.Fatalf("events HTTP %d",response.Code)}
 body:=response.Body.String()
 for _,phrase:=range []string{
  "Registry image event (image/platform scope)","Remote registry digest changed",
  "Webhook pending (0 attempts)","Previous:","Current:",
  "image/platform scoped","no notifier is configured",
 }{
  if !strings.Contains(body,phrase){t.Errorf("events missing %q",phrase)}
 }
 if strings.Contains(body,"<td> / </td>"){t.Fatal("event has empty project/service row")}
 if strings.Contains(body,"hello-js / web")||strings.Contains(body,"hello-py / web"){
  t.Fatal("image-scoped event was misattributed to a watcher")
 }
}

func TestSettingsDeepLinkHighlightsCorrectService(t *testing.T){
 h,db,_,_:=setup(t)
 ctx:=context.Background()
 if err:=db.Snapshot(ctx,[]store.Container{{
  ID:"test-container",Name:"app-web-1",Project:"app",Service:"web",
  Image:"nginx:latest",ImageID:"sha256:config",State:"running",
  ImageOS:"linux",ImageArchitecture:"amd64",
  MonitorLabels:map[string]string{"com.docklantern.monitor":"true"},
 }});err!=nil{t.Fatal(err)}
 response:=request(h,"GET","/settings?project=app&service=web","",true)
 if response.Code!=200{t.Fatal(response.Code)}
 if !strings.Contains(response.Body.String(),"service-control selected-service"){
  t.Fatal("deep link failed to highlight service")
 }
}
