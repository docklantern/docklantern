package management

import (
 "context"
 "net/http"
 "net/url"
 "path/filepath"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/store"
)

func TestBeta4WorkspaceRoutesAndDeepLinks(t *testing.T) {
 h,db,project,_:=setup(t)
 ctx:=context.Background()
 registration:=store.Registration{
  Name:"site",ProjectDir:project,Files:[]string{filepath.Join(project,"compose.yaml")},
  Services:[]string{"web","worker"},
 }
 if err:=db.Register(ctx,registration);err!=nil{t.Fatal(err)}
 for _,service:=range []string{"web","worker"}{
  p:=store.MonitorPolicy{Project:"site",Service:service,Image:"nginx:latest",Platform:"linux/amd64",IntervalSeconds:300}
  if err:=db.SetMonitorPolicy(ctx,p);err!=nil{t.Fatal(err)}
 }
 containers:=[]store.Container{
  {ID:"web-1",Project:"site",Service:"web",Image:"nginx:latest",State:"running",ImageOS:"linux",ImageArchitecture:"amd64",Health:"healthy"},
  {ID:"worker-1",Project:"site",Service:"worker",Image:"nginx:latest",State:"running",ImageOS:"linux",ImageArchitecture:"amd64"},
  {ID:"cache-1",Project:"other",Service:"cache",Image:"redis:7",State:"running",ImageOS:"linux",ImageArchitecture:"amd64"},
 }
 if err:=db.Snapshot(ctx,containers);err!=nil{t.Fatal(err)}
 policies,err:=db.MonitorPolicies(ctx);if err!=nil{t.Fatal(err)}
 if len(policies)!=2{t.Fatalf("want two monitoring policies: %+v",policies)}
 id:=presentation.ImageID(presentation.ImageIdentity{Reference:policies[0].Image,Platform:policies[0].Platform})
 if strings.ContainsAny(id,"/+?&="){t.Fatalf("unsafe encoded image ID %q",id)}
 paths:=[]struct{url,stringContains string}{
  {"/images","Watched images"},
  {"/images/"+id,"Associated services"},
  {"/services","Services"},
  {"/services/site/web","site / web"},
  {"/services/other/cache","Not enrolled"},
 }
 for _,tc:=range paths{
  t.Run(tc.url,func(t *testing.T){
   if got:=request(h,"GET",tc.url,"",false).Code;got!=http.StatusUnauthorized{t.Fatalf("unauthenticated route returned %d",got)}
   r:=request(h,"GET",tc.url,"",true)
   if r.Code!=http.StatusOK{t.Fatalf("GET %s: %d: %s",tc.url,r.Code,r.Body.String())}
   body:=r.Body.String()
   if !strings.Contains(body,tc.stringContains){t.Errorf("missing %q",tc.stringContains)}
   for _,header:=range []string{"Content-Security-Policy","Cache-Control","X-Content-Type-Options"}{
    if r.Header().Get(header)==""{t.Errorf("missing %s",header)}
   }
   if strings.Count(body,`aria-current="page"`) < 1 {t.Error("no active sidebar or breadcrumb")}
   if !strings.Contains(body,`<a class="skip-link" href="#main">Skip to content</a>`){t.Error("missing keyboard skip link")}
  })
 }
 img:=request(h,"GET","/images","",true).Body.String()
 if count:=strings.Count(img,`>View image</a>`);count!=1{
  t.Fatalf("shared image should be one row; got %d: %s",count,img)
 }
 detail:=request(h,"GET","/images/"+id,"",true).Body.String()
 if !strings.Contains(detail,`href="/services/site/web"`) || !strings.Contains(detail,`href="/services/site/worker"`){
  t.Fatalf("image detail missing associated service links: %s",detail)
 }
 service:=request(h,"GET","/services/site/web","",true).Body.String()
 if !strings.Contains(service,`href="/images/`+id+`"`){
  t.Fatalf("service detail missing image deep link: %s",service)
 }
 if !strings.Contains(service,`href="/settings?project=site&amp;service=web#service-monitoring"`){
  t.Fatalf("service settings deep link not preserved: %s",service)
 }
 if got:=request(h,"GET","/api/v1/monitor-policies","",true).Code;got!=http.StatusOK{
  t.Errorf("API regression %d",got)
 }
}

func TestBeta4WorkspaceRoutesRejectUnknownAndMalformedIdentities(t *testing.T){
 h,_,_,_:=setup(t)
 for _,path:=range []string{
  "/images/invalid!",
  "/images/"+presentation.ImageID(presentation.ImageIdentity{Reference:"registry-1.docker.io/library/not-there:latest",Platform:"linux/amd64"}),
  "/services/nope/web",
  "/services/nope/web/extra",
 }{
  r:=request(h,"GET",path,"",true)
  if r.Code!=http.StatusNotFound{t.Errorf("expected 404 for %s, got %d",path,r.Code)}
 }
 if got:=request(h,"POST","/images","",true).Code;got!=http.StatusMethodNotAllowed{
  t.Errorf("new read-only image route accepted POST: %d",got)
 }
 if got:=request(h,"POST","/services","",true).Code;got!=http.StatusMethodNotAllowed{
  t.Errorf("new read-only services route accepted POST: %d",got)
 }
}

func TestBeta4WorkspaceTemplatesEscapeDockerMetadata(t *testing.T){
 h,db,_,_:=setup(t)
 ctx:=context.Background()
 payload:=`<img src=x onerror=alert(1)>`
 if err:=db.Snapshot(ctx,[]store.Container{{
  ID:"t1",Project:"demo",Service:"web",Image:payload,State:"running",
 }});err!=nil{t.Fatal(err)}
 body:=request(h,"GET","/services","",true).Body.String()
 if strings.Contains(body,payload){t.Fatal("unescaped Docker inventory image rendered as HTML")}
 if !strings.Contains(body,"&lt;img"){t.Fatal("expected escaped image representation")}
}

func TestBeta4ExistingSettingsAndEventsStillWork(t *testing.T){
 h,_,_,_:=setup(t)
 values:=url.Values{"project":{"example"},"service":{"web"}}
 settings:=request(h,"GET","/settings?"+values.Encode(),"",true)
 if settings.Code!=http.StatusOK || !strings.Contains(settings.Body.String(),`action="/settings/monitoring/service"`){
  t.Fatalf("monitoring settings forms not accessible: %d",settings.Code)
 }
 events:=request(h,"GET","/events?kind=remote_digest_changed","",true)
 if events.Code!=http.StatusOK || !strings.Contains(events.Body.String(),`action="/events"`){
  t.Fatalf("activity filter not accessible: %d",events.Code)
 }
}
