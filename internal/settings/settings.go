// Package settings exposes CSRF-protected monitoring-only administration.
package settings

import (
 "context"
 "errors"
 "html/template"
 "net/http"
 "strings"

 "github.com/docklantern/docklantern/internal/auth"
 "github.com/docklantern/docklantern/internal/poc"
 "github.com/docklantern/docklantern/internal/store"
)

type Backend interface {
 Projects(context.Context)([]store.Registration,error)
 MonitorPolicies(context.Context)([]store.MonitorPolicy,error)
 Register(context.Context,store.Registration)error
 SetMonitorPolicy(context.Context,store.MonitorPolicy)error
}
type Handler struct {
 Backend Backend
 Guard *auth.Guard
 Roots []string
}
type pageData struct {
 CSRF string
 Projects []store.Registration
 Policies []store.MonitorPolicy
 Notice string
}
var tmpl=template.Must(template.New("settings").Parse(pageHTML))
const pageHTML=`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Settings · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head>
<body><main class="main settings-page"><header class="topbar"><div><p class="eyebrow">DOCKER COMPOSE / ADMINISTRATION</p><h1>Monitoring settings</h1><p class="muted">Register Compose projects and configure read-only registry checks.</p></div><a class="text-action" href="/">← Overview</a></header>
{{if .Notice}}<p role="status" class="notice">{{.Notice}}</p>{{end}}
<section class="section panel settings-panel"><h2>Register project</h2><p class="muted">All paths must be absolute and inside an operator-approved source root. Registration never enables deployments.</p>
<form method="post" action="/settings/projects"><input type="hidden" name="csrf_token" value="{{.CSRF}}">
<label>Compose project name<input name="name" required maxlength="128" placeholder="example"></label>
<label>Project directory<input name="project_dir" required placeholder="/srv/apps/example"></label>
<label>Compose files (one absolute path per line)<textarea name="files" rows="3" required placeholder="/srv/apps/example/compose.yaml"></textarea></label>
<label>Services (comma-separated)<input name="services" required placeholder="web, worker"></label>
<label>Environment files (one absolute path per line; optional)<textarea name="env_files" rows="2"></textarea></label>
<label>Profiles (comma-separated; optional)<input name="profiles"></label>
<button type="submit">Register for monitoring</button></form></section>
<section class="section panel settings-panel"><h2>Configure watched service</h2><p class="muted">Only explicitly registered services are eligible. Select pause to stop registry checks.</p>
<form method="post" action="/settings/policies"><input type="hidden" name="csrf_token" value="{{.CSRF}}">
<label>Project<input name="project" required placeholder="example"></label>
<label>Service<input name="service" required placeholder="web"></label>
<label>Tagged image<input name="image" required placeholder="ghcr.io/org/app:latest"></label>
<label>Platform<input name="platform" required value="linux/amd64"></label>
<label>Interval (seconds, 60–86400)<input name="interval_seconds" type="number" min="60" max="86400" value="300" required></label>
<label class="checkline"><input name="paused" type="checkbox" value="true"> Pause monitoring</label>
<button type="submit">Save monitoring policy</button></form></section>
<section class="section panel settings-panel"><h2>Registered projects</h2><div class="table-scroll"><table><thead><tr><th>Project</th><th>Services</th><th>Directory</th></tr></thead><tbody>{{range .Projects}}<tr><td>{{.Name}}</td><td>{{range .Services}}{{.}} {{end}}</td><td class="mono">{{.ProjectDir}}</td></tr>{{else}}<tr><td colspan="3" class="table-empty">None registered yet.</td></tr>{{end}}</tbody></table></div>
<h2>Existing schedules</h2><div class="table-scroll"><table><thead><tr><th>Project/service</th><th>Image</th><th>Interval</th><th>State</th></tr></thead><tbody>{{range .Policies}}<tr><td>{{.Project}} / {{.Service}}</td><td class="mono">{{.Image}}</td><td>{{.IntervalSeconds}}s</td><td>{{if .Paused}}Paused{{else}}Scheduled{{end}}</td></tr>{{else}}<tr><td colspan="4" class="table-empty">None configured yet.</td></tr>{{end}}</tbody></table></div></section>
<footer>No automatic image pulls, deployment controls, or credential storage.</footer></main></body></html>`

func (h Handler) Routes(mux *http.ServeMux){
 mux.HandleFunc("GET /settings",h.index)
 mux.HandleFunc("POST /settings/projects",h.register)
 mux.HandleFunc("POST /settings/policies",h.policy)
}
func (h Handler) index(w http.ResponseWriter,r *http.Request){
 projects,e:=h.Backend.Projects(r.Context());if e!=nil{http.Error(w,"Settings unavailable",500);return}
 policies,e:=h.Backend.MonitorPolicies(r.Context());if e!=nil{http.Error(w,"Settings unavailable",500);return}
 w.Header().Set("Content-Type","text/html; charset=utf-8")
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("Referrer-Policy","no-referrer")
 w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'self'; img-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
 _=tmpl.Execute(w,pageData{CSRF:h.Guard.Token(),Projects:projects,Policies:policies,Notice:notice(r.URL.Query().Get("saved"))})
}
func notice(code string)string{switch code{case "project":return "Project registered for monitoring.";case "policy":return "Monitoring policy saved.";default:return ""}}
func (h Handler) prepare(w http.ResponseWriter,r *http.Request)bool{
 if !strings.HasPrefix(r.Header.Get("Content-Type"),"application/x-www-form-urlencoded"){http.Error(w,"Form encoding required",415);return false}
 r.Body=http.MaxBytesReader(w,r.Body,32<<10)
 if !h.Guard.CheckCSRF(r){http.Error(w,"CSRF validation failed",403);return false}
 return true
}
func (h Handler) register(w http.ResponseWriter,r *http.Request){
 if !h.prepare(w,r){return}
 p:=store.Registration{
  Name:strings.TrimSpace(r.PostFormValue("name")),
  ProjectDir:strings.TrimSpace(r.PostFormValue("project_dir")),
  Files:lines(r.PostFormValue("files")),
  EnvFiles:lines(r.PostFormValue("env_files")),
  Services:comma(r.PostFormValue("services")),
  Profiles:comma(r.PostFormValue("profiles")),
  Enabled:false,
 }
 if len(h.Roots)==0{http.Error(w,"Project registration not enabled",403);return}
 v:=poc.Registration{Name:p.Name,ProjectDir:p.ProjectDir,Files:p.Files,EnvFiles:p.EnvFiles,Services:p.Services,Profiles:p.Profiles}
 if err:=v.Validate(h.Roots);err!=nil{http.Error(w,"Invalid project registration",400);return}
 if err:=h.Backend.Register(r.Context(),p);err!=nil{http.Error(w,"Registration could not be saved",400);return}
 http.Redirect(w,r,"/settings?saved=project",http.StatusSeeOther)
}
func(h Handler) policy(w http.ResponseWriter,r *http.Request){
 if !h.prepare(w,r){return}
 seconds,e:=parseInterval(r.PostFormValue("interval_seconds"));if e!=nil{http.Error(w,"Invalid polling interval",400);return}
 paused:=r.PostFormValue("paused")=="true"
 p:=store.MonitorPolicy{Project:strings.TrimSpace(r.PostFormValue("project")),Service:strings.TrimSpace(r.PostFormValue("service")),Image:strings.TrimSpace(r.PostFormValue("image")),Platform:strings.TrimSpace(r.PostFormValue("platform")),IntervalSeconds:seconds,Paused:paused}
 if e=h.Backend.SetMonitorPolicy(r.Context(),p);e!=nil{http.Error(w,"Invalid monitoring policy",400);return}
 http.Redirect(w,r,"/settings?saved=policy",http.StatusSeeOther)
}
func lines(s string)[]string{
 var out []string;for _,line:=range strings.Split(s,"\n"){line=strings.TrimSpace(line);if line!=""{out=append(out,line)}}
 return out
}
func comma(s string)[]string{
 var out []string;for _,v:=range strings.Split(s,","){v=strings.TrimSpace(v);if v!=""{out=append(out,v)}}
 return out
}
func parseInterval(s string)(int,error){
 if len(s)==0||len(s)>5{return 0,errors.New("invalid interval")}
 n:=0;for _,ch:=range s{if ch<'0'||ch>'9'{return 0,errors.New("invalid interval")};n=n*10+int(ch-'0')}
 if n<60||n>86400{return 0,errors.New("interval out of range")}
 return n,nil
}
