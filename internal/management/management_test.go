package management

import (
 "context"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/auth"
 "github.com/docklantern/docklantern/internal/store"
)
func setup(t *testing.T)(http.Handler,*store.Store,string,*auth.Guard){
 t.Helper()
 root:=t.TempDir()
 project:=filepath.Join(root,"project")
 if err:=os.MkdirAll(project,0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(project,"compose.yaml"),[]byte("services: {}"),0600);err!=nil{t.Fatal(err)}
 db,e:=store.Open(filepath.Join(root,"db.sqlite"));if e!=nil{t.Fatal(e)}
 t.Cleanup(func(){_ =db.Close()})
 guard,e:=auth.New("admin",strings.Repeat("very-secure-demo-password-",2));if e!=nil{t.Fatal(e)}
 return Handler(db,guard,[]string{root}),db,project,guard
}
func request(h http.Handler,method,path,body string,authenticated bool)*httptest.ResponseRecorder{
 r:=httptest.NewRequest(method,"http://127.0.0.1:8787"+path,strings.NewReader(body))
 if method==http.MethodPost{r.Header.Set("Content-Type","application/x-www-form-urlencoded");r.Header.Set("Origin","http://127.0.0.1:8787")}
 if authenticated{r.SetBasicAuth("admin",strings.Repeat("very-secure-demo-password-",2))}
 w:=httptest.NewRecorder();h.ServeHTTP(w,r);return w
}
func TestPrivateRoutesAndSettings(t *testing.T){
 h,db,project,g:=setup(t)
 for _,path:=range []string{"/","/settings","/api/v1/projects","/api/v1/registry-checks","/assets/dashboard.css"}{
  if got:=request(h,"GET",path,"",false).Code;got!=401{t.Fatalf("unauth %s: %d",path,got)}
  if got:=request(h,"GET",path,"",true).Code;got!=200{t.Fatalf("authorized %s: %d",path,got)}
 }
 if request(h,"GET","/healthz","",false).Code!=200{t.Fatal("health not public")}
 if got:=request(h,"POST","/settings/projects","",false).Code;got!=401{t.Fatalf("unauthorized mutation: %d",got)}
 form:=url.Values{
  "name":{"demo"},
  "project_dir":{project},
  "files":{filepath.Join(project,"compose.yaml")},
  "services":{"web,worker"},
 }
 if got:=request(h,"POST","/settings/projects",form.Encode(),true).Code;got!=403{t.Fatalf("no CSRF: %d",got)}
 form.Set("csrf_token",g.Token())
 if got:=request(h,"POST","/settings/projects",form.Encode(),true).Code;got!=303{t.Fatalf("registration: %d",got)}
 projects,e:=db.Projects(context.Background());if e!=nil||len(projects)!=1||projects[0].Enabled{t.Fatalf("registration %+v %v",projects,e)}
 policy:=url.Values{"csrf_token":{g.Token()},"project":{"demo"},"service":{"web"},"image":{"nginx:1.27-alpine"},"platform":{"linux/amd64"},"interval_seconds":{"300"}}
 if got:=request(h,"POST","/settings/policies",policy.Encode(),true).Code;got!=303{t.Fatalf("policy: %d",got)}
 policies,e:=db.MonitorPolicies(context.Background());if e!=nil||len(policies)!=1||policies[0].Image!="registry-1.docker.io/library/nginx:1.27-alpine"{t.Fatalf("policy %+v %v",policies,e)}
 policy.Set("service","unregistered")
 if got:=request(h,"POST","/settings/policies",policy.Encode(),true).Code;got!=400{t.Fatalf("unapproved policy: %d",got)}
}
func TestRejectPathEscapeAndCrossOrigin(t *testing.T){
 h,db,project,g:=setup(t)
 outside:=t.TempDir()
 file:=filepath.Join(outside,"compose.yaml")
 if err:=os.WriteFile(file,[]byte("services: {}"),0600);err!=nil{t.Fatal(err)}
 values:=url.Values{"csrf_token":{g.Token()},"name":{"escape"},"project_dir":{outside},"files":{file},"services":{"web"}}
 if got:=request(h,"POST","/settings/projects",values.Encode(),true).Code;got!=400{t.Fatalf("allowed path escape: %d",got)}
 values.Set("project_dir",project);values.Set("files",filepath.Join(project,"compose.yaml"))
 req:=httptest.NewRequest("POST","http://127.0.0.1:8787/settings/projects",strings.NewReader(values.Encode()))
 req.Header.Set("Content-Type","application/x-www-form-urlencoded");req.Header.Set("Origin","https://malicious.example");req.SetBasicAuth("admin",strings.Repeat("very-secure-demo-password-",2))
 w:=httptest.NewRecorder();h.ServeHTTP(w,req)
 if w.Code!=403{t.Fatalf("cross-origin post accepted: %d",w.Code)}
 projects,e:=db.Projects(context.Background());if e!=nil||len(projects)!=0{t.Fatal("unexpected registration")}
}
