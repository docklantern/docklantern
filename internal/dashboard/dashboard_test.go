package dashboard

import (
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/store"
)

type fakeReader struct {
 projects []store.Registration
 containers []store.Container
 checks []store.RegistryCheck
 policies []store.MonitorPolicy
 recentEvents []store.Event
 latestChanges []store.Event
 workers []store.WorkerStatus
 fail bool
}
func(f fakeReader)Projects(context.Context)([]store.Registration,error){
 if f.fail{return nil,errors.New("private SQLite error")}
 return f.projects,nil
}
func(f fakeReader)Containers(context.Context)([]store.Container,error){return f.containers,nil}
func(f fakeReader)RegistryChecks(context.Context)([]store.RegistryCheck,error){return f.checks,nil}
func(f fakeReader)MonitorPolicies(context.Context)([]store.MonitorPolicy,error){return f.policies,nil}
func(f fakeReader)Events(context.Context,int)([]store.Event,error){return f.recentEvents,nil}
func(f fakeReader)LatestImageChangeEvents(context.Context)([]store.Event,error){return f.latestChanges,nil}
func(f fakeReader)WorkerStatuses(context.Context)([]store.WorkerStatus,error){return f.workers,nil}

func TestDashboardImageOverviewAndEscaping(t *testing.T){
 now:=time.Now().UTC()
 malicious:=`<script>alert("XSS")</script>`
 image:="ghcr.io/team/"+malicious+":latest"
 change:=store.Event{Kind:"remote_digest_changed",Image:image,Platform:"linux/amd64",OccurredAt:now.Add(-time.Hour).Format(time.RFC3339Nano)}
 f:=fakeReader{
  projects:[]store.Registration{{Name:"app",ProjectDir:"/srv/app",Services:[]string{"web"}}},
  containers:[]store.Container{{Project:"app",Service:"web",Image:image,State:"running",Health:"healthy"}},
  checks:[]store.RegistryCheck{{Image:image,Platform:"linux/amd64",Status:"changed",CheckedAt:now.Add(-time.Minute).Format(time.RFC3339),PlatformDigest:"sha256:"+strings.Repeat("a",64)}},
  policies:[]store.MonitorPolicy{{Project:"app",Service:"web",Image:image,Platform:"linux/amd64",IntervalSeconds:300}},
  recentEvents:[]store.Event{change},
  latestChanges:[]store.Event{change},
  workers:[]store.WorkerStatus{{Name:"registry_monitor",State:"ok",LastHeartbeatAt:now.Format(time.RFC3339)}},
 }
 w:=httptest.NewRecorder()
 Handler(f).ServeHTTP(w,httptest.NewRequest("GET","/",nil))
 if w.Code!=200{t.Fatalf("status=%d %s",w.Code,w.Body.String())}
 body:=w.Body.String()
 for _,expected:=range []string{
  "Image Overview","Watched Images","Remote Changes","Deployment Differences",
  "Monitoring Errors","Images to Review","Recent Image Activity",
  "Background Checks","registry_monitor","View all activity",
  "Remote digest changed in the last 24 hours",
 }{
  if !strings.Contains(body,expected){t.Errorf("missing overview text %q",expected)}
 }
 if strings.Contains(body,"Container overview") ||
  strings.Contains(body,"Service schedules") ||
  strings.Contains(body,"Registered sources") ||
  strings.Contains(body,"Latest registry checks"){
  t.Fatal("old container-first sections must not dominate Overview")
 }
 if strings.Contains(body,malicious){t.Fatal("untrusted Docker metadata rendered as HTML")}
 if !strings.Contains(body,"&lt;script&gt;"){t.Fatal("expected escaped image metadata")}
 imageID:=presentation.ImageID(presentation.ImageIdentity{Reference:image,Platform:"linux/amd64"})
 if !strings.Contains(body,`href="/images/`+imageID+`"`){t.Fatal("review must deep-link to associated Image Details")}
 if got:=w.Header().Get("Content-Security-Policy");!strings.Contains(got,"default-src 'none'"){t.Fatalf("unexpected CSP: %q",got)}
 if got:=w.Header().Get("Cache-Control");got!="no-store"{t.Fatalf("unexpected cache policy: %q",got)}
}

func TestDashboardEmptyAndErrors(t *testing.T){
 for _,tc:=range []struct{f fakeReader;status int;contains string}{
  {fakeReader{},200,"No watched images currently meet the review criteria"},
  {fakeReader{fail:true},500,"Dashboard unavailable"},
 }{
  w:=httptest.NewRecorder()
  Handler(tc.f).ServeHTTP(w,httptest.NewRequest("GET","/",nil))
  if w.Code!=tc.status||!strings.Contains(w.Body.String(),tc.contains){t.Fatalf("status %d: %s",w.Code,w.Body.String())}
  if strings.Contains(w.Body.String(),"private SQLite error"){t.Fatal("leaked error")}
 }
}

func TestDashboardMethodAndAssets(t *testing.T){
 h:=Handler(fakeReader{})
 for _,tc:=range []struct{method,path string;want int}{
  {"POST","/",http.StatusMethodNotAllowed},
  {"GET","/does-not-exist",http.StatusNotFound},
  {"GET","/assets/dashboard.css",http.StatusOK},
  {"POST","/assets/dashboard.css",http.StatusMethodNotAllowed},
 }{
  w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest(tc.method,tc.path,nil))
  if w.Code!=tc.want{t.Errorf("%s %s got %d expected %d",tc.method,tc.path,w.Code,tc.want)}
 }
}
