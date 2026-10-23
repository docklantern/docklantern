// Package dashboard renders the monitoring inventory as read-only server-side HTML.
package dashboard

import (
 "context"
 "embed"
 "html/template"
 "net/http"
 "strings"
 "time"

 "github.com/docklantern/docklantern/internal/store"
 "github.com/docklantern/docklantern/internal/updates"
)

//go:embed assets/dashboard.html assets/dashboard.css
var assets embed.FS

var page = template.Must(template.New("dashboard.html").Funcs(template.FuncMap{
 "short": func(s string) string { if len(s)>28 {return s[:28]+"…"};return s },
 "badge": func(s string) string { switch strings.ToLower(s) {case "running","healthy","unchanged":return "ok";case "changed","unhealthy","exited":return "warn";case "paused","first_seen","stale_inventory","stale_registry","unknown","not_checked":return "muted";default:return "neutral"} },
 "when": func(s string) string {if s==""{return "Not yet"};t,e:=time.Parse(time.RFC3339,s);if e!=nil{return s};return t.UTC().Format("2006-01-02 15:04 UTC")},
}).ParseFS(assets,"assets/dashboard.html"))

type Reader interface {
 Projects(context.Context)([]store.Registration,error)
 Containers(context.Context)([]store.Container,error)
 RegistryChecks(context.Context)([]store.RegistryCheck,error)
 MonitorPolicies(context.Context)([]store.MonitorPolicy,error)
}

type view struct {
 Projects []project
 Unregistered []store.Container
 Checks []store.RegistryCheck
 Policies []store.MonitorPolicy
 Assessments []updates.Assessment
 ProjectCount int
 ContainerCount int
 RunningCount int
 MonitoredCount int
 ChangedCount int
 CheckedAt string
}
type project struct {
 Registration store.Registration
 Containers []store.Container
 Policies []store.MonitorPolicy
}
func collect(ctx context.Context,s Reader)(view,error){
 registrations,e:=s.Projects(ctx);if e!=nil{return view{},e}
 containers,e:=s.Containers(ctx);if e!=nil{return view{},e}
 checks,e:=s.RegistryChecks(ctx);if e!=nil{return view{},e}
 policies,e:=s.MonitorPolicies(ctx);if e!=nil{return view{},e}
 v:=view{Checks:checks,Policies:policies,Assessments:updates.AssessFresh(registrations,containers,policies,checks,time.Now().UTC()),ProjectCount:len(registrations),ContainerCount:len(containers),MonitoredCount:len(policies)}
 idx:=make(map[string]int,len(registrations))
 for _,r:=range registrations{idx[r.Name]=len(v.Projects);v.Projects=append(v.Projects,project{Registration:r})}
 for _,c:=range containers{
  if c.State=="running"{v.RunningCount++}
  if i,ok:=idx[c.Project];ok {v.Projects[i].Containers=append(v.Projects[i].Containers,c)}else{v.Unregistered=append(v.Unregistered,c)}
 }
 for _,p:=range policies{if i,ok:=idx[p.Project];ok{v.Projects[i].Policies=append(v.Projects[i].Policies,p)}}
 for _,c:=range checks{if c.Status=="changed"{v.ChangedCount++}}
 return v,nil
}
func Handler(s Reader) http.Handler{
 mux:=http.NewServeMux()
 css,e:=assets.ReadFile("assets/dashboard.css");if e!=nil{panic(e)}
 mux.HandleFunc("GET /assets/dashboard.css",func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Content-Type","text/css; charset=utf-8")
  w.Header().Set("Cache-Control","public, max-age=3600")
  w.Header().Set("X-Content-Type-Options","nosniff")
  _,_=w.Write(css)
 })
 mux.HandleFunc("GET /",func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/"{http.NotFound(w,r);return}
  v,e:=collect(r.Context(),s);if e!=nil{http.Error(w,"Dashboard unavailable",http.StatusInternalServerError);return}
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("X-Content-Type-Options","nosniff")
  w.Header().Set("Referrer-Policy","no-referrer")
  w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
  if e=page.ExecuteTemplate(w,"dashboard.html",v);e!=nil{return}
 })
 return mux
}
