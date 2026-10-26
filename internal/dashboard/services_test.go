package dashboard

import (
 "context"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/store"
)

func TestServiceFirstDashboardWithLabelEnrollmentAndRegistryEvents(t *testing.T){
 ctx:=context.Background()
 db,err:=store.Open(filepath.Join(t.TempDir(),"service.sqlite"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 first:=store.Container{
  ID:"one",Name:"hello-js-web-1",Project:"hello-js",Service:"web",
  Image:"ghcr.io/example/hello-js:latest",ImageID:"sha256:config1",State:"running",Health:"healthy",
  ImageOS:"linux",ImageArchitecture:"amd64",
  MonitorLabels:map[string]string{"com.docklantern.monitor":"true","com.docklantern.interval":"10m"},
 }
 replica:=first;replica.ID="two";replica.Name="hello-js-web-2"
 unlabeled:=store.Container{ID:"three",Name:"other-worker-1",Project:"other",Service:"worker",
   Image:"redis:7",ImageID:"sha256:config2",State:"exited",ImageOS:"linux",ImageArchitecture:"amd64"}
 if err:=db.Snapshot(ctx,[]store.Container{first,replica,unlabeled});err!=nil{t.Fatal(err)}
 p,err:=db.MonitorPolicies(ctx)
 if err!=nil||len(p)!=1||p[0].IntervalSeconds!=600{t.Fatalf("label watch %+v %v",p,err)}
 image,platform:=p[0].Image,p[0].Platform
 for i,digest:=range []string{"sha256:first","sha256:changed","sha256:changed"}{
  check,err:=db.RecordRegistryCheck(ctx,image,platform,"sha256:index",digest)
  if err!=nil{t.Fatal(err)}
  expected:=[]string{"first_seen","changed","unchanged"}[i]
  if check.Status!=expected{t.Fatalf("registry stage %d: %+v",i,check)}
 }
 events,err:=db.Events(ctx,20)
 if err!=nil||len(events)!=1||events[0].Project!=""||events[0].Service!=""{
  t.Fatalf("expected exactly one image-scoped digest event: %+v %v",events,err)
 }
 w:=httptest.NewRecorder()
 Handler(db).ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/",nil))
 if w.Code!=http.StatusOK{t.Fatalf("GET / status %d: %s",w.Code,w.Body.String())}
 body:=w.Body.String()
 for _,expected:=range []string{
  "Services and monitoring status","hello-js / web","other / worker",
  "Running (2/2)","Health: healthy","Stopped (1 containers)",
  "Docker label","600s","Edit monitoring","View related events",
  "/settings?project=hello-js&amp;service=web#service-monitoring",
  "/events?search=ghcr.io%2Fexample%2Fhello-js%3Alatest",
  "Remote registry digest changed","Registry image event (image/platform scope)",
  "Webhook pending (0 attempts)","Remote digest unchanged",
  "No registered projects yet",
 }{
  if !strings.Contains(body,expected){t.Errorf("service dashboard missing %q",expected)}
 }
 if strings.Contains(body,"available update")||strings.Contains(body,"newer release available"){
  t.Fatal("remote digest was incorrectly labeled as a newer release")
 }
 if strings.Count(body,"<strong>hello-js / web</strong>")!=1{
  t.Fatalf("service row duplicated across replicas: %d",strings.Count(body,"<strong>hello-js / web</strong>"))
 }
}

func TestServiceDashboardDoesNotTrustUnescapedDockerMetadata(t *testing.T){
 ctx:=context.Background()
 db,err:=store.Open(filepath.Join(t.TempDir(),"escape.sqlite"));if err!=nil{t.Fatal(err)};defer db.Close()
 hostile:=`<script>alert(1)</script>`
 c:=store.Container{ID:"one",Name:hostile,Project:"proj",Service:"web",
  Image:"nginx:1",ImageID:"sha256:config",ImageOS:"linux",ImageArchitecture:"amd64",
  State:"running",Health:hostile}
 if err:=db.Snapshot(ctx,[]store.Container{c});err!=nil{t.Fatal(err)}
 w:=httptest.NewRecorder();Handler(db).ServeHTTP(w,httptest.NewRequest("GET","/",nil))
 if w.Code!=200{t.Fatal(w.Code)}
 if strings.Contains(w.Body.String(),hostile){t.Fatal("trusted raw Docker HTML")}
 if !strings.Contains(w.Body.String(),"&lt;script&gt;"){t.Fatal("expected escaped user data")}
}
