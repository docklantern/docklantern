package dashboard

import (
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/store"
)

type fakeReader struct {
 projects []store.Registration
 containers []store.Container
 checks []store.RegistryCheck
 policies []store.MonitorPolicy
 fail bool
}
func(f fakeReader)Projects(context.Context)([]store.Registration,error){if f.fail{return nil,errors.New("private SQLite error")};return f.projects,nil}
func(f fakeReader)Containers(context.Context)([]store.Container,error){return f.containers,nil}
func(f fakeReader)RegistryChecks(context.Context)([]store.RegistryCheck,error){return f.checks,nil}
func(f fakeReader)MonitorPolicies(context.Context)([]store.MonitorPolicy,error){return f.policies,nil}

func TestDashboardOverviewAndEscaping(t *testing.T){
 malicious:=`<script>alert("XSS")</script>`
 f:=fakeReader{
  projects:[]store.Registration{{Name:"app",ProjectDir:"/srv/app",Services:[]string{"web"}}},
  containers:[]store.Container{{Project:"app",Service:"web",Name:malicious,Image:"ghcr.io/team/app:latest",State:"running",Health:"healthy"}, {Project:"other",Service:"worker",Image:"busybox:latest",State:"exited"}},
  checks:[]store.RegistryCheck{{Image:"ghcr.io/team/app:latest",Platform:"linux/amd64",Status:"changed",PlatformDigest:"sha256:"+strings.Repeat("a",64)}},
  policies:[]store.MonitorPolicy{{Project:"app",Service:"web",Image:"ghcr.io/team/app:latest",Platform:"linux/amd64",IntervalSeconds:300,Paused:true,LastError:malicious}},
 }
 h:=Handler(f)
 w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest("GET","/",nil))
 if w.Code!=200{t.Fatalf("status=%d %s",w.Code,w.Body.String())}
 body:=w.Body.String()
 for _,expected:=range []string{"Container overview","Compose projects","Latest registry checks","Service schedules","ghcr.io/team/app:latest","Discovered but not registered","Paused","changed"}{
  if !strings.Contains(body,expected){t.Errorf("missing %q",expected)}
 }
 if strings.Contains(body,malicious){t.Fatal("untrusted Docker metadata was rendered as HTML")}
 if !strings.Contains(body,"&lt;script&gt;"){t.Fatal("expected escaped metadata")}
 if got:=w.Header().Get("Content-Security-Policy");!strings.Contains(got,"default-src 'none'"){t.Fatalf("unexpected CSP: %q",got)}
 if got:=w.Header().Get("Cache-Control");got!="no-store"{t.Fatalf("unexpected cache policy: %q",got)}
}
func TestDashboardEmptyAndErrors(t *testing.T){
 for _,tc:=range []struct{f fakeReader; status int;contains string}{
  {fakeReader{},200,"No registered projects yet"},
  {fakeReader{fail:true},500,"Dashboard unavailable"},
 }{
  w:=httptest.NewRecorder();Handler(tc.f).ServeHTTP(w,httptest.NewRequest("GET","/",nil))
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
