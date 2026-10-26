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

func monitoringForm(token,project,service,action string, seconds string)string{
 return url.Values{
  "csrf_token":{token},"project":{project},"service":{service},
  "action":{action},"interval_seconds":{seconds},
 }.Encode()
}

func TestLiveMonitorGUIFormsAndCSRF(t *testing.T){
 h,db,_,g:=setup(t)
 ctx:=context.Background()
 docker:=store.Container{
  ID:"id1",Name:"demo-web-1",Project:"demo",Service:"web",
  Image:"nginx:latest",ImageID:"sha256:local",
  ImageOS:"linux",ImageArchitecture:"amd64",State:"running",
 }
 if err:=db.Snapshot(ctx,[]store.Container{docker});err!=nil{t.Fatal(err)}
 settings:=request(h,"GET","/settings","",true)
 if settings.Code!=http.StatusOK{t.Fatalf("GET settings %d",settings.Code)}
 for _,expected:=range []string{
  "Monitoring defaults","Service monitoring","Global default",
  "Default check interval","Enable in GUI",
  `action="/settings/monitoring/default"`,
  `action="/settings/monitoring/service"`,
  `value="300"`,
  "demo / web",
 } {
  if !strings.Contains(settings.Body.String(),expected){
   t.Errorf("missing settings UI %q",expected)
  }
 }
 noToken:=request(h,"POST","/settings/monitoring/default","interval_seconds=120",true)
 if noToken.Code!=http.StatusForbidden{t.Fatalf("no CSRF accepted %d",noToken.Code)}
 global:=url.Values{"csrf_token":{g.Token()},"interval_seconds":{"120"}}.Encode()
 if got:=request(h,"POST","/settings/monitoring/default",global,true);got.Code!=http.StatusSeeOther{
  t.Fatalf("global update status %d: %s",got.Code,got.Body.String())
 }
 if got,err:=db.DefaultMonitorInterval(ctx);err!=nil||got!=120{t.Fatalf("default not saved: %d %v",got,err)}
 enable:=monitoringForm(g.Token(),"demo","web","enable","")
 if got:=request(h,"POST","/settings/monitoring/service",enable,true);got.Code!=http.StatusSeeOther{
  t.Fatalf("GUI enrollment status %d: %s",got.Code,got.Body.String())
 }
 override:=monitoringForm(g.Token(),"demo","web","override","180")
 if got:=request(h,"POST","/settings/monitoring/service",override,true);got.Code!=http.StatusSeeOther{
  t.Fatalf("override status %d: %s",got.Code,got.Body.String())
 }
 p,err:=db.MonitorPolicies(ctx)
 if err!=nil||len(p)!=1||p[0].IntervalSeconds!=180{t.Fatalf("override %+v %v",p,err)}
 api:=request(h,"GET","/api/v1/monitoring/settings","",true)
 if api.Code!=http.StatusOK{t.Fatalf("settings API %d: %s",api.Code,api.Body.String())}
 for _,term:=range []string{`"default_interval_seconds":120`,`"effective_interval_seconds":180`,`"interval_source":"GUI override"`,`"enrollment_source":"GUI"`}{
  if !strings.Contains(api.Body.String(),term){t.Errorf("API response missing %s",term)}
 }
 settings=request(h,"GET","/settings","",true)
 for _,expected:=range []string{"GUI override","180s","Reset override","Pause"}{
  if !strings.Contains(settings.Body.String(),expected){t.Errorf("missing updated state %q",expected)}
 }
 if got:=request(h,"POST","/settings/monitoring/service",monitoringForm(g.Token(),"demo","web","pause",""),true);got.Code!=303{
  t.Fatalf("pause %d",got.Code)
 }
 due,err:=db.DueMonitorPolicies(ctx,time.Now().Add(time.Hour),10)
 if err!=nil||len(due)!=0{t.Fatalf("paused watch due %+v %v",due,err)}
 if got:=request(h,"POST","/settings/monitoring/service",monitoringForm(g.Token(),"demo","web","reset",""),true);got.Code!=303{
  t.Fatalf("reset %d",got.Code)
 }
 p,_=db.MonitorPolicies(ctx)
 if p[0].IntervalSeconds!=120{t.Fatalf("reset did not return to global default %+v",p)}
 if got:=request(h,"POST","/settings/monitoring/service",monitoringForm(g.Token(),"demo","web","resume",""),true);got.Code!=303{
  t.Fatalf("resume %d",got.Code)
 }
 due,_=db.DueMonitorPolicies(ctx,time.Now().Add(time.Minute),10)
 if len(due)!=1{t.Fatal("resume not scheduled")}
 if got:=request(h,"POST","/settings/monitoring/service",monitoringForm(g.Token(),"demo","web","disable",""),true);got.Code!=303{
  t.Fatalf("disable %d",got.Code)
 }
 due,_=db.DueMonitorPolicies(ctx,time.Now().Add(time.Hour),10)
 if len(due)!=0{t.Fatal("disabled service scheduled")}
}

func TestMonitorSettingsInvalidActionsAndLabelOptOut(t *testing.T){
 h,db,_,g:=setup(t)
 ctx:=context.Background()
 docker:=store.Container{
  ID:"id2",Name:"blocked-web-1",Project:"blocked",Service:"web",
  Image:"nginx:latest",ImageID:"sha256:local",ImageOS:"linux",
  ImageArchitecture:"amd64",State:"running",
  MonitorLabels:map[string]string{"com.docklantern.monitor":"false"},
 }
 if err:=db.Snapshot(ctx,[]store.Container{docker});err!=nil{t.Fatal(err)}
 for _,tc:=range []struct{path,form string}{
  {"/settings/monitoring/default",url.Values{"csrf_token":{g.Token()},"interval_seconds":{"59"}}.Encode()},
  {"/settings/monitoring/service",monitoringForm(g.Token(),"blocked","web","enable","")},
  {"/settings/monitoring/service",monitoringForm(g.Token(),"blocked","web","override","20")},
  {"/settings/monitoring/service",monitoringForm(g.Token(),"blocked","web","delete-everything","")},
  {"/settings/monitoring/service",monitoringForm(g.Token(),"missing","web","resume","")},
 }{
  r:=request(h,"POST",tc.path,tc.form,true)
  if r.Code!=http.StatusBadRequest{t.Errorf("%s accepted bad input: %d",tc.path,r.Code)}
 }
 noAuth:=request(h,"POST","/settings/monitoring/service",monitoringForm(g.Token(),"blocked","web","pause",""),false)
 if noAuth.Code!=http.StatusUnauthorized{t.Fatalf("authentication bypassed: %d",noAuth.Code)}
}
