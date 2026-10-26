package management

import (
 "context"
 "fmt"
 "net/http"
 "net/url"
 "strconv"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/store"
)

func TestBeta4SettingsSectionsRetentionAndExistingAdmin(t *testing.T){
 h,db,_,guard:=setup(t)
 ctx:=context.Background()
 // A real discovered service is needed before its legacy management form
 // exists; a fresh empty installation correctly renders no service actions.
 if err:=db.Snapshot(ctx,[]store.Container{{
  ID:"demo-web-1",Project:"demo",Service:"web",Image:"nginx:latest",
  ImageOS:"linux",ImageArchitecture:"amd64",State:"running",
 }});err!=nil{t.Fatal(err)}
 initial,err:=db.NotificationSettings(ctx)
 if err!=nil{t.Fatal(err)}
 page:=request(h,http.MethodGet,"/settings","",true)
 if page.Code!=200{t.Fatal(page.Code)}
 html:=page.Body.String()
 for _,want:=range []string{
  "<h1>Settings</h1>","Monitoring defaults","Notification preferences",
  "History and retention","Advanced / Legacy Configuration",
  `action="/settings/retention"`,`action="/settings/events/prune"`,
  `action="/settings/notifications"`,`action="/settings/monitoring/default"`,
  `action="/settings/projects"`,`action="/settings/policies"`,
  `action="/settings/monitoring/service"`,`href="/services"`,
  `list="monitoring-interval-presets"`,
 }{
  if !strings.Contains(html,want){t.Errorf("Settings missing %q",want)}
 }
 if got:=request(h,http.MethodPost,"/settings/retention","retention_days=17",true).Code;got!=403{t.Fatalf("retention bypassed CSRF: %d",got)}
 if got:=request(h,http.MethodPost,"/settings/retention",url.Values{
  "csrf_token":{guard.Token()},"retention_days":{"17"},
 }.Encode(),false).Code;got!=401{t.Fatalf("unauthenticated retention accepted: %d",got)}
 for _,invalid:=range []string{"0","3651","bad",""}{
  form:=url.Values{"csrf_token":{guard.Token()},"retention_days":{invalid}}
  if got:=request(h,http.MethodPost,"/settings/retention",form.Encode(),true).Code;got!=400{
   t.Errorf("invalid retention value %q accepted: %d",invalid,got)
  }
 }
 form:=url.Values{"csrf_token":{guard.Token()},"retention_days":{"17"}}
 response:=request(h,http.MethodPost,"/settings/retention",form.Encode(),true)
 if response.Code!=303||response.Header().Get("Location")!="/settings?saved=retention#history-retention"{
  t.Fatalf("unexpected retention redirect: %d %s",response.Code,response.Header().Get("Location"))
 }
 updated,err:=db.NotificationSettings(ctx)
 if err!=nil{t.Fatal(err)}
 if updated.RetentionDays!=17{t.Fatalf("retention not saved: %+v",updated)}
 initial.RetentionDays=17
 if updated!=initial{t.Fatalf("retention-only form unexpectedly altered notifier settings: %+v vs %+v",updated,initial)}
 done:=request(h,http.MethodGet,"/settings?saved=retention","",true)
 if !strings.Contains(done.Body.String(),"History retention period saved."){t.Error("missing accessible confirmation")}
 if got:=request(h,"POST","/settings/events/prune",url.Values{"csrf_token":{guard.Token()}}.Encode(),true).Code;got!=400{
  t.Fatalf("prune without confirmation was permitted: %d",got)
 }
}

func TestBeta4ActivityTimelineScopedLinksAndKeysetPages(t *testing.T){
 h,db,_,_:=setup(t)
 ctx:=context.Background()
 shared:="ghcr.io/example/shared:latest"
 blocked:="ghcr.io/example/unwatched:latest"
 containers:=[]store.Container{}
 for _,project:=range []string{"hello-js","hello-go"}{
  containers=append(containers,store.Container{
   ID:project+"-1",Project:project,Service:"web",Image:shared,State:"running",
   ImageOS:"linux",ImageArchitecture:"amd64",
   MonitorLabels:map[string]string{"com.docklantern.monitor":"true"},
  })
 }
 if err:=db.Snapshot(ctx,containers);err!=nil{t.Fatal(err)}
 policies,err:=db.MonitorPolicies(ctx)
 if err!=nil||len(policies)!=2{t.Fatalf("expected shared-image monitoring: %+v %v",policies,err)}
 identity:=presentation.ImageIdentity{Reference:policies[0].Image,Platform:policies[0].Platform}
 // 52 retained digest changes plus initial observation force two event pages.
 for i:=0;i<54;i++{
  digest:="sha256:"+fmt.Sprintf("%064x",i+1)
  if _,err:=db.RecordRegistryCheck(ctx,identity.Reference,identity.Platform,"sha256:index",digest);err!=nil{t.Fatal(err)}
 }
 first:=request(h,http.MethodGet,"/events?kind=remote_digest_changed&search=shared","",true)
 if first.Code!=200{t.Fatalf("Activity page failed: %d %s",first.Code,first.Body.String())}
 body:=first.Body.String()
 for _,required:=range []string{
  "<h1>Activity</h1>","Monitoring history","Digest changes","Previous:","Current:",
  "Webhook pending (0 attempts)","image/platform scoped",
  `value="remote_digest_changed" selected`,`name="search" maxlength="128" value="shared"`,
  "Older events","Registry image event (image/platform scope)",
 }{
  if !strings.Contains(body,required){t.Errorf("Activity missing %q",required)}
 }
 id:=presentation.ImageID(identity)
 if !strings.Contains(body,`href="/images/`+id+`"`){t.Fatal("Activity missing associated image detail link")}
 if strings.Contains(body,"hello-js / web")||strings.Contains(body,"hello-go / web"){
  t.Fatal("image-scoped event falsely attributed to one Compose service")
 }
 if count:=strings.Count(body,`class="activity-entry"`);count!=50{t.Errorf("expected 50 first-page event cards, got %d",count)}
 // Find the valid ID cursor from the retained store; a change event must be
 // scoped to this image, and subsequent pages must be strictly older.
 rawEvents,err:=db.FilterEvents(ctx,store.EventFilter{Kind:"remote_digest_changed",Search:"shared",Limit:51})
 if err!=nil||len(rawEvents)!=51{t.Fatalf("store event pagination setup: %d %v",len(rawEvents),err)}
 expectedCursor:=rawEvents[49].ID
 if !strings.Contains(body,`name="before" value="`+strconv.FormatInt(expectedCursor,10)+`"`){
  t.Fatal("Activity pagination did not point to last rendered event")
 }
 next:=request(h,http.MethodGet,"/events?kind=remote_digest_changed&search=shared&before="+strconv.FormatInt(expectedCursor,10),"",true)
 if next.Code!=200 || strings.Count(next.Body.String(),`class="activity-entry"`)!=3{
  t.Fatalf("Activity second page lost events: %d",next.Code)
 }
 if strings.Contains(next.Body.String(),`name="before"`){t.Fatal("last page incorrectly offers older cursor")}
 // Changing the observation result to unchanged must not generate another
 // digest change event.
 if _,err:=db.RecordRegistryCheck(ctx,identity.Reference,identity.Platform,"sha256:index",fmt.Sprintf("%064x",54));err!=nil{
  t.Fatal(err)
 }
 if got:=request(h,http.MethodGet,"/events?kind=bad","",true).Code;got!=400{t.Fatalf("invalid event type accepted: %d",got)}
 if got:=request(h,http.MethodGet,"/events?before=bad","",true).Code;got!=400{t.Fatalf("invalid cursor accepted: %d",got)}
 if got:=request(h,http.MethodGet,"/events?search=%3Cscript%3E","",true);strings.Contains(got.Body.String(),"<script>"){
  t.Fatal("Activity reflected unsafe unescaped search")
 }
 // Unwatched image events must not acquire a fabricated Image Details link.
 if _,err:=db.RecordRegistryCheck(ctx,blocked,"linux/amd64","sha256:index","sha256:first");err!=nil{t.Fatal(err)}
 if _,err:=db.RecordRegistryCheck(ctx,blocked,"linux/amd64","sha256:index","sha256:next");err!=nil{t.Fatal(err)}
 unwatched:=request(h,http.MethodGet,"/events?search=unwatched","",true)
 if unwatched.Code!=200 || strings.Contains(unwatched.Body.String(),`href="/images/`){
  t.Fatal("unwatched historical event linked to missing image detail")
 }
 if got:=request(h,"GET","/events","",false).Code;got!=http.StatusUnauthorized{
  t.Fatalf("Activity accessible without login: %d",got)
 }
}
