package management

import (
 "context"
 "net/http"
 "net/url"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/store"
)

func TestBeta4ServicesWorkspaceAndMonitoringActions(t *testing.T){
 h,db,_,guard:=setup(t)
 ctx:=context.Background()
 containers:=[]store.Container{
  {ID:"site-web",Name:"site-web-1",Project:"site",Service:"web",Image:"nginx:latest",
   ImageOS:"linux",ImageArchitecture:"amd64",State:"running",Health:"healthy"},
  {ID:"site-web-2",Name:"site-web-2",Project:"site",Service:"web",Image:"nginx:latest",
   ImageOS:"linux",ImageArchitecture:"amd64",State:"running",Health:"healthy"},
  {ID:"site-worker",Name:"site-worker-1",Project:"site",Service:"worker",Image:"alpine:latest",
   ImageOS:"linux",ImageArchitecture:"amd64",State:"running",
   MonitorLabels:map[string]string{"com.docklantern.monitor":"true","com.docklantern.interval":"10m"}},
  {ID:"data-db",Name:"data-db-1",Project:"data",Service:"db",Image:"postgres:17",
   ImageOS:"linux",ImageArchitecture:"amd64",State:"running",
   MonitorLabels:map[string]string{"com.docklantern.monitor":"false"}},
 }
 if err:=db.Snapshot(ctx,containers);err!=nil{t.Fatal(err)}
 page:=request(h,"GET","/services","",true)
 if page.Code!=200{t.Fatalf("services status %d: %s",page.Code,page.Body.String())}
 body:=page.Body.String()
 for _,term:=range []string{"Compose services","site","data","Not enrolled","Disabled by label","worker","2 / 2 running","10m"}{
  // 10m is displayed as the effective monitoring interval in seconds.
  if term=="10m"{term="600s"}
  if !strings.Contains(body,term){t.Errorf("missing %q in Services workspace",term)}
 }
 if strings.Count(body,`href="/services/site/web"`)!=1{
  t.Fatalf("replicas should group into one service row")
 }
 search:=request(h,"GET","/services?q=POSTGRES&monitoring=disabled_by_label","",true)
 if search.Code!=200 || !strings.Contains(search.Body.String(),`href="/services/data/db"`) || strings.Contains(search.Body.String(),`href="/services/site/web"`){
  t.Fatalf("combined search/filter not applied: %d",search.Code)
 }
 for _,path:=range []string{"/services?monitoring=write","/services?q="+strings.Repeat("a",129)}{
  if got:=request(h,"GET",path,"",true).Code;got!=400{t.Errorf("invalid filter %s accepted: %d",path,got)}
 }
 blocked:=request(h,"GET","/services/data/db","",true)
 if blocked.Code!=200{t.Fatal(blocked.Code)}
 if !strings.Contains(blocked.Body.String(),"explicitly disabled by the Docker Compose label") ||
  strings.Contains(blocked.Body.String(),`value="enable">Enable monitoring`) ||
  strings.Contains(blocked.Body.String(),`value="override">Save override`){
  t.Fatal("explicit monitor=false is not protected in UI")
 }
 disabled:=url.Values{
  "csrf_token":{guard.Token()},"project":{"data"},"service":{"db"},"action":{"enable"},
  "return_to":{"/services/data/db"},
 }
 if got:=request(h,"POST","/settings/monitoring/service",disabled.Encode(),true).Code;got!=400{
  t.Fatalf("explicit Docker opt-out bypassed via hand-crafted POST: %d",got)
 }
 details:=request(h,"GET","/services/site/web","",true)
 if details.Code!=200{t.Fatalf("service detail status %d",details.Code)}
 for _,term:=range []string{
  "Monitoring configuration","Enrollment source","Effective interval","Running containers",
  "Image and deployment evidence","Enable monitoring",`action="/settings/monitoring/service"`,
  `name="csrf_token"`,`name="return_to" value="/services/site/web"`,
 }{
  if !strings.Contains(details.Body.String(),term){t.Errorf("missing service detail term %q",term)}
 }
 if !strings.Contains(details.Header().Get("Content-Security-Policy"),"form-action 'self'"){
  t.Fatal("read-only workspace CSP blocks its own CSRF-protected settings forms")
 }
 post:=func(action,seconds,returnTo string)*http.Response{
  t.Helper()
  f:=url.Values{"csrf_token":{guard.Token()},"project":{"site"},"service":{"web"},"action":{action},"return_to":{returnTo}}
  if seconds!=""{f.Set("interval_seconds",seconds)}
  rec:=request(h,"POST","/settings/monitoring/service",f.Encode(),true)
  return rec.Result()
 }
 if got:=request(h,"POST","/settings/monitoring/service",url.Values{"project":{"site"},"service":{"web"},"action":{"enable"},"return_to":{"/services/site/web"}}.Encode(),true).Code;got!=403{
  t.Fatalf("service control bypassed CSRF: %d",got)
 }
 enabled:=post("enable","","/services/site/web")
 if enabled.StatusCode!=303 || enabled.Header.Get("Location")!="/services/site/web?saved=service#monitoring-controls"{
  t.Fatalf("enable redirect: %d %s",enabled.StatusCode,enabled.Header.Get("Location"))
 }
 if got:=request(h,"GET","/services/site/web?saved=service","",true);got.Code!=200 || !strings.Contains(got.Body.String(),"Service monitoring settings saved."){
  t.Fatal("return-to-details confirmation missing")
 }
 if got:=post("override","180","/services/site/web");got.StatusCode!=303{t.Fatalf("override: %d",got.StatusCode)}
 policies,err:=db.MonitorPolicies(ctx)
 if err!=nil{t.Fatal(err)}
 var overrideFound bool
 for _,p:=range policies{if p.Project=="site" && p.Service=="web"{overrideFound=p.IntervalSeconds==180}}
 if !overrideFound{t.Fatal("GUI interval override was not persisted")}
 if got:=request(h,"GET","/services/site/web","",true);got.Code!=200 || !strings.Contains(got.Body.String(),"Reset to label/default"){
  t.Fatal("reset action not offered following override")
 }
 if got:=post("pause","","/services/site/web");got.StatusCode!=303{t.Fatalf("pause: %d",got.StatusCode)}
 due,err:=db.DueMonitorPolicies(ctx,time.Now().Add(time.Hour),20)
 if err!=nil{t.Fatal(err)}
 for _,p:=range due{if p.Project=="site"&&p.Service=="web"{t.Fatal("paused policy remains due")}}
 paused:=request(h,"GET","/services/site/web","",true).Body.String()
 if !strings.Contains(paused,">Resume monitoring</button>"){t.Fatal("resume control absent")}
 if got:=post("reset","","/services/site/web");got.StatusCode!=303{t.Fatalf("reset: %d",got.StatusCode)}
 policies,_=db.MonitorPolicies(ctx)
 for _,p:=range policies{if p.Project=="site" && p.Service=="web" && p.IntervalSeconds!=300{t.Fatal("reset did not restore global default")}}
 if got:=post("resume","","/services/site/web");got.StatusCode!=303{t.Fatalf("resume: %d",got.StatusCode)}
 if got:=post("disable","","https://evil.example/");got.StatusCode!=303 || got.Header.Get("Location")!="/settings?saved=service#service-monitoring"{
  t.Fatalf("untrusted redirect accepted: %d %q",got.StatusCode,got.Header.Get("Location"))
 }
 if got:=request(h,"GET","/services/site/web","",false).Code;got!=http.StatusUnauthorized{
  t.Fatalf("detail page leaked without authentication: %d",got)
 }
}

func TestBeta4LabelEnrollmentProvenanceOnServiceDetails(t *testing.T){
 h,db,_,_:=setup(t)
 if err:=db.Snapshot(context.Background(),[]store.Container{{
  ID:"demo1",Project:"demo",Service:"web",Image:"ghcr.io/example/web:latest",State:"running",
  ImageOS:"linux",ImageArchitecture:"amd64",
  MonitorLabels:map[string]string{"com.docklantern.monitor":"true","com.docklantern.interval":"10m"},
 }});err!=nil{t.Fatal(err)}
 page:=request(h,"GET","/services/demo/web","",true)
 if page.Code!=200{t.Fatal(page.Code)}
 html:=page.Body.String()
 for _,term:=range []string{"Docker label interval","600 seconds","Docker label","Pause monitoring"}{
  if !strings.Contains(html,term){t.Errorf("missing label provenance %q",term)}
 }
 if strings.Contains(html,`value="disable">Disable GUI enrollment`){
  t.Fatal("label enrolled policy should not expose GUI disable")
 }
}
