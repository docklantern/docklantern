package management

import (
    "context"
    "net/http"
    "net/http/httptest"
    "net/url"
    "strings"
    "testing"
    "time"

    "github.com/docklantern/docklantern/internal/presentation"
    "github.com/docklantern/docklantern/internal/store"
)

func TestCheckNowEndpointsAuthenticationCSRFAndPersistentSchedule(t *testing.T) {
    h,db,_,guard:=setup(t)
    ctx:=context.Background()
    c:=store.Container{ID:"web-1",Project:"site",Service:"web",
        Image:"nginx:latest",ImageOS:"linux",ImageArchitecture:"amd64",State:"running",
        MonitorLabels:map[string]string{"com.docklantern.monitor":"true"}}
    if err:=db.Snapshot(ctx,[]store.Container{c});err!=nil{t.Fatal(err)}
    policies,err:=db.MonitorPolicies(ctx)
    if err!=nil||len(policies)!=1{t.Fatalf("missing policy %+v %v",policies,err)}
    // A prior check more than one minute ago and a future next-due time.
    if err:=db.FinishMonitorCheck(ctx,policies[0],time.Now().UTC().Add(-2*time.Minute),nil);err!=nil{t.Fatal(err)}
    form:=url.Values{"project":{"site"},"service":{"web"},"csrf_token":{guard.Token()}}
    path:="/settings/monitoring/check-now/service"
    if x:=request(h,"POST",path,form.Encode(),false);x.Code!=http.StatusUnauthorized{
        t.Fatalf("missing authentication: %d",x.Code)
    }
    form.Del("csrf_token")
    if x:=request(h,"POST",path,form.Encode(),true);x.Code!=http.StatusForbidden{
        t.Fatalf("missing csrf: %d",x.Code)
    }
    form.Set("csrf_token",guard.Token())
    req:=httptest.NewRequest("POST","http://127.0.0.1:8787"+path,strings.NewReader(form.Encode()))
    req.SetBasicAuth("admin",strings.Repeat("very-secure-demo-password-",2))
    req.Header.Set("Origin","https://evil.example")
    req.Header.Set("Content-Type","application/x-www-form-urlencoded")
    rec:=httptest.NewRecorder();h.ServeHTTP(rec,req)
    if rec.Code!=http.StatusForbidden{t.Fatalf("cross origin accepted: %d",rec.Code)}
    before,_:=db.MonitorPolicies(ctx)
    if before[0].Generation!=policies[0].Generation{t.Fatal("bad request mutated policy")}
    response:=request(h,"POST",path,form.Encode(),true)
    if response.Code!=http.StatusSeeOther||
      response.Header().Get("Location")!="/services/site/web?check=scheduled#monitoring-controls"{
        t.Fatalf("request did not schedule work: %d %s",response.Code,response.Header().Get("Location"))
    }
    after,_:=db.MonitorPolicies(ctx)
    if after[0].Generation<=before[0].Generation || after[0].Image!=before[0].Image {
        t.Fatal("request failed to preserve image or update due schedule")
    }
    second:=request(h,"POST",path,form.Encode(),true)
    if second.Code!=http.StatusSeeOther||
      !strings.Contains(second.Header().Get("Location"),"check=deferred_recent") {
        t.Fatalf("repeat request bypassed throttle: %d %s",second.Code,second.Header().Get("Location"))
    }
    if code:=request(h,"GET",path,"",true).Code;code!=http.StatusNotFound && code!=http.StatusMethodNotAllowed{
        t.Fatalf("GET unexpectedly reached the POST-only action: %d",code)
    }
}

func TestCheckNowImageIDAndBlockedServices(t *testing.T) {
    h,db,_,guard:=setup(t)
    ctx:=context.Background()
    items:=[]store.Container{
      {ID:"a1",Project:"a",Service:"web",Image:"nginx:latest",
       ImageOS:"linux",ImageArchitecture:"amd64",State:"running",
       MonitorLabels:map[string]string{"com.docklantern.monitor":"true"}},
      {ID:"b1",Project:"b",Service:"web",Image:"nginx:latest",
       ImageOS:"linux",ImageArchitecture:"amd64",State:"running",
       MonitorLabels:map[string]string{"com.docklantern.monitor":"false"}},
    }
    if err:=db.Snapshot(ctx,items);err!=nil{t.Fatal(err)}
    path:="/settings/monitoring/check-now/image"
    identity:=presentation.ImageIdentity{Reference:"registry-1.docker.io/library/nginx:latest",Platform:"linux/amd64"}
    id:=presentation.ImageID(identity)
    form:=url.Values{"image_id":{id},"csrf_token":{guard.Token()}}
    got:=request(h,"POST",path,form.Encode(),true)
    if got.Code!=303||got.Header().Get("Location")!="/images/"+id+"?check=already_due"{
        t.Fatalf("known due image result: %d %s",got.Code,got.Header().Get("Location"))
    }
    form.Set("image_id","not-base64-identity")
    if x:=request(h,"POST",path,form.Encode(),true);x.Code!=http.StatusBadRequest{
        t.Fatalf("unvalidated image identity accepted: %d",x.Code)
    }
    form.Set("image_id",presentation.ImageID(presentation.ImageIdentity{
        Reference:"registry-1.docker.io/library/nginx:latest",Platform:"linux/arm64"}))
    if x:=request(h,"POST",path,form.Encode(),true);x.Code!=http.StatusConflict{
        t.Fatalf("unwatched image accepted: %d",x.Code)
    }
    form.Set("image_id",id)
    form.Del("csrf_token")
    if x:=request(h,"POST",path,form.Encode(),true);x.Code!=http.StatusForbidden{
        t.Fatalf("image POST bypassed CSRF: %d",x.Code)
    }
    service:=url.Values{"csrf_token":{guard.Token()},"project":{"b"},"service":{"web"}}
    if x:=request(h,"POST","/settings/monitoring/check-now/service",service.Encode(),true);x.Code!=http.StatusConflict{
        t.Fatalf("explicit label opt-out accepted: %d",x.Code)
    }
    service.Set("project","unknown")
    if x:=request(h,"POST","/settings/monitoring/check-now/service",service.Encode(),true);x.Code!=http.StatusConflict{
        t.Fatalf("unknown service accepted: %d",x.Code)
    }
}
