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

func TestBeta4ImagesFilteringAndExactRegistryHistory(t *testing.T) {
 h,db,project,_:=setup(t)
 ctx:=context.Background()
 if err:=db.Register(ctx,store.Registration{
  Name:"demo",ProjectDir:project,Files:[]string{filepath.Join(project,"compose.yaml")},
  Services:[]string{"web","worker"},
 });err!=nil{t.Fatal(err)}
 for _,p:=range []store.MonitorPolicy{
  {Project:"demo",Service:"web",Image:"nginx:latest",Platform:"linux/amd64",IntervalSeconds:300},
  {Project:"demo",Service:"worker",Image:"redis:7",Platform:"linux/amd64",IntervalSeconds:300},
 }{
  if err:=db.SetMonitorPolicy(ctx,p);err!=nil{t.Fatal(err)}
 }
 policies,err:=db.MonitorPolicies(ctx)
 if err!=nil{t.Fatal(err)}
 names:=map[string]string{}
 for _,p:=range policies{names[p.Service]=p.Image}
 d1:="sha256:"+strings.Repeat("a",64)
 d2:="sha256:"+strings.Repeat("b",64)
 for _,name:=range []string{names["web"],names["worker"]}{
  if _,err:=db.RecordRegistryCheck(ctx,name,"linux/amd64",d1,d1);err!=nil{t.Fatal(err)}
  if _,err:=db.RecordRegistryCheck(ctx,name,"linux/amd64",d2,d2);err!=nil{t.Fatal(err)}
 }
 if _,err:=db.RecordRegistryCheck(ctx,names["web"],"linux/amd64",d2,d2);err!=nil{t.Fatal(err)}
 id:=presentation.ImageID(presentation.ImageIdentity{Reference:names["web"],Platform:"linux/amd64"})
 page:=request(h,"GET","/images?q=nginx&registry=unchanged&sort=name","",true)
 if page.Code!=http.StatusOK{t.Fatalf("image list status %d: %s",page.Code,page.Body.String())}
 body:=page.Body.String()
 if !strings.Contains(body,"Last recorded change") || !strings.Contains(body,">View image</a>"){
  t.Fatalf("missing registry history summary or details link: %s",body)
 }
 if strings.Contains(body,">redis:7<") || strings.Count(body,">View image</a>")!=1{
  t.Fatalf("search/filter incorrectly includes another image: %s",body)
 }
 changed:=request(h,"GET","/images?registry=recorded_change_24h","",true)
 if changed.Code!=200 || strings.Count(changed.Body.String(),">View image</a>")!=2{
  t.Fatalf("historical recent-change filter ignored following unchanged check: %d: %s",changed.Code,changed.Body.String())
 }
 detail:=request(h,"GET","/images/"+id,"",true)
 if detail.Code!=200{t.Fatalf("detail page unavailable: %d",detail.Code)}
 html:=detail.Body.String()
 if !strings.Contains(html,"Registry evidence") || !strings.Contains(html,"Index digest") ||
    !strings.Contains(html,"Registry history") || !strings.Contains(html,"Remote registry digest changed"){
  t.Fatalf("image detail missing evidence or events: %s",html)
 }
 if strings.Contains(html,names["worker"]){
  t.Fatalf("unrelated image data leaked into exact-image history: %s",html)
 }
 if !strings.Contains(html,d2){t.Fatal("full immutable registry digest missing from details")}
 for _,input:=range []string{
  "/images?registry=upgrade_available",
  "/images?sort=untrusted",
  "/images?deployment=auto_update",
  "/images/"+id+"?before=-5",
  "/images/"+id+"?before=not-a-number",
 }{
  if got:=request(h,"GET",input,"",true).Code;got!=400{t.Errorf("invalid query %s returned %d, expected 400",input,got)}
 }
 if got:=request(h,"GET","/images/"+id,"",false).Code;got!=http.StatusUnauthorized{
  t.Errorf("image details bypassed authentication: %d",got)
 }
 if got:=request(h,"POST","/images/"+id,url.Values{}.Encode(),true).Code;got!=http.StatusMethodNotAllowed{
  t.Errorf("image detail allowed mutation: %d",got)
 }
}
