// Package settings exposes CSRF-protected monitoring-only administration.
package settings

import (
 "context"
 "errors"
 "html/template"
 "net/http"
 "strconv"
 "time"
 "strings"

 "github.com/docklantern/docklantern/internal/auth"
 "github.com/docklantern/docklantern/internal/layout"
 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/poc"
 "github.com/docklantern/docklantern/internal/store"
)

type Backend interface {
 Projects(context.Context)([]store.Registration,error)
 MonitorPolicies(context.Context)([]store.MonitorPolicy,error)
 Containers(context.Context)([]store.Container,error)
 Register(context.Context,store.Registration)error
 SetMonitorPolicy(context.Context,store.MonitorPolicy)error
 NotificationSettings(context.Context)(store.NotificationSettings,error)
 SetNotificationSettings(context.Context,store.NotificationSettings)error
 PruneDeliveredEvents(context.Context,time.Time)(int64,error)
 FilterEvents(context.Context,store.EventFilter)([]store.Event,error)
 DefaultMonitorInterval(context.Context)(int,error)
 SetDefaultMonitorInterval(context.Context,int)error
 MonitorServiceViews(context.Context)([]store.MonitorServiceView,error)
 SetServiceIntervalOverride(context.Context,string,string,*int)error
 SetServiceGUIEnrollment(context.Context,string,string,bool)error
 SetServicePaused(context.Context,string,string,bool)error
}
type Handler struct {
 Backend Backend
 Guard *auth.Guard
 Roots []string
}
type pageData struct {
 ActivePage string
 MainClass string
 CSRF string
 Projects []store.Registration
 Policies []store.MonitorPolicy
 Services []store.MonitorServiceView
 DefaultInterval int
 Notice string
 Notifications store.NotificationSettings
 Discovered []store.Container
 SelectedProject string
 SelectedService string
 SuggestedProject string
 SuggestedService string
 SuggestedImage string
}
var tmpl=template.Must(layout.WithShell(template.Must(template.New("settings").Parse(pageHTML))))
const pageHTML=`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Settings · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head>
<body>{{template "shell-start" .}}<header class="topbar"><div><p class="eyebrow">DOCKER COMPOSE / ADMINISTRATION</p><h1>Monitoring settings</h1><p class="muted">Manage opt-in services, schedules and read-only registry checks.</p></div><a class="text-action" href="/">← Overview</a></header>
{{if .Notice}}<p role="status" class="notice">{{.Notice}}</p>{{end}}
<section class="section panel settings-panel" id="monitoring-defaults">
<h2>Monitoring defaults</h2>
<p class="muted">New label- or GUI-enrolled services inherit this interval unless they have a Docker interval label or GUI override. Changes apply to inheriting watches without restarting a container.</p>
<form method="post" action="/settings/monitoring/default">
<input type="hidden" name="csrf_token" value="{{.CSRF}}">
<label>Default check interval (seconds, 60–86400)
<input type="number" name="interval_seconds" min="60" max="86400" value="{{.DefaultInterval}}" required></label>
<button type="submit">Save global default</button>
</form></section>
<section class="section panel settings-panel" id="service-monitoring">
<h2>Service monitoring</h2>
<p class="muted">Opt in an unlabeled discovered service, or override its interval. Effective setting priority: GUI override → Docker label → global default. Original Compose project registration is not required for monitoring.</p>
{{range .Services}}
<div class="service-control{{if and (eq .Project $.SelectedProject) (eq .Service $.SelectedService)}} selected-service{{end}}">
<div class="service-control-head">
<div><h3>{{.Project}} / {{.Service}}</h3>
<p class="muted">{{if .Image}}<span class="mono">{{.Image}}</span>{{else}}No validated monitoring policy{{end}}</p></div>
<span class="pill">{{.EnrollmentSource}}</span>
</div>
<p class="muted">Discovery: {{.State}}{{if .ValidationError}} — {{.ValidationError}}{{end}}
{{if .Paused}} · Paused{{end}}{{if .NextCheckAt}} · Next scheduled: {{.NextCheckAt}}{{end}}</p>
<p><strong>Effective interval: {{.EffectiveInterval}}s</strong> ({{.IntervalSource}})
{{if .LabelInterval}} · Docker label: {{.LabelInterval}}s{{end}}
{{if .GUIOverride}} · GUI override: {{.GUIOverride}}s{{end}}</p>
<div class="service-control-actions">
{{if .CanGUIEnable}}
<form method="post" action="/settings/monitoring/service">
<input type="hidden" name="csrf_token" value="{{$.CSRF}}">
<input type="hidden" name="project" value="{{.Project}}">
<input type="hidden" name="service" value="{{.Service}}">
<button type="submit" name="action" value="enable">Enable in GUI</button></form>
{{end}}
{{if .CanGUIDisable}}
<form method="post" action="/settings/monitoring/service">
<input type="hidden" name="csrf_token" value="{{$.CSRF}}">
<input type="hidden" name="project" value="{{.Project}}">
<input type="hidden" name="service" value="{{.Service}}">
<button type="submit" name="action" value="disable">Disable GUI enrollment</button></form>
{{end}}
{{if .CanEditInterval}}
<form method="post" action="/settings/monitoring/service">
<input type="hidden" name="csrf_token" value="{{$.CSRF}}">
<input type="hidden" name="project" value="{{.Project}}">
<input type="hidden" name="service" value="{{.Service}}">
<label>Custom interval (seconds)
<input type="number" name="interval_seconds" min="60" max="86400" value="{{.EffectiveInterval}}" required></label>
<button type="submit" name="action" value="override">Save override</button>
<button type="submit" name="action" value="reset" formnovalidate>Reset override</button>
</form>
{{end}}
{{if .CanPause}}
<form method="post" action="/settings/monitoring/service">
<input type="hidden" name="csrf_token" value="{{$.CSRF}}">
<input type="hidden" name="project" value="{{.Project}}">
<input type="hidden" name="service" value="{{.Service}}">
<button type="submit" name="action" value="pause">Pause</button></form>
{{end}}
{{if .CanResume}}
<form method="post" action="/settings/monitoring/service">
<input type="hidden" name="csrf_token" value="{{$.CSRF}}">
<input type="hidden" name="project" value="{{.Project}}">
<input type="hidden" name="service" value="{{.Service}}">
<button type="submit" name="action" value="resume">Resume</button></form>
{{end}}
</div></div>
{{else}}<p class="muted">No Compose services discovered yet. Enable Docker discovery to show services here.</p>{{end}}
</section>
<section class="section panel settings-panel"><h2>Discovered services</h2><p class="muted">Discovered containers are also listed in the monitoring controls above. Prefill the legacy source registration form below only if you need to retain an existing manually configured project.</p>
<div class="table-scroll"><table><thead><tr><th>Project</th><th>Service</th><th>Image</th><th>Choose</th></tr></thead><tbody>
{{range .Discovered}}<tr><td>{{.Project}}</td><td>{{.Service}}</td><td class="mono">{{.Image}}</td><td><form method="get" action="/settings"><input type="hidden" name="project" value="{{.Project}}"><input type="hidden" name="service" value="{{.Service}}"><button type="submit">Prefill</button></form></td></tr>
{{else}}<tr><td colspan="4" class="table-empty">No services discovered. Enable the optional discovery proxy and inventory scanning to populate this list.</td></tr>{{end}}
</tbody></table></div></section>
<section class="section panel settings-panel"><h2>Notification preferences</h2><p class="muted">The webhook sender must be configured separately using an HTTPS URL and a signing key file. No secrets are stored here. Muted events remain in history and pending delivery until re-enabled.</p>
<form method="post" action="/settings/notifications"><input type="hidden" name="csrf_token" value="{{.CSRF}}">
<label class="checkline"><input type="checkbox" name="digest_changes" value="on" {{if .Notifications.DigestChanges}}checked{{end}}> Remote digest changed</label>
<label class="checkline"><input type="checkbox" name="registry_failures" value="on" {{if .Notifications.RegistryFailures}}checked{{end}}> Registry check failed</label>
<label class="checkline"><input type="checkbox" name="registry_recovery" value="on" {{if .Notifications.RegistryRecovery}}checked{{end}}> Registry check recovered</label>
<label class="checkline"><input type="checkbox" name="prolonged_outage" value="on" {{if .Notifications.ProlongedOutage}}checked{{end}}> Prolonged registry outage</label>
<label>Consecutive failures to alert (2–100)<input type="number" name="outage_failures" min="2" max="100" value="{{.Notifications.OutageFailures}}" required></label>
<label>Event retention days (1–3650)<input type="number" name="retention_days" min="1" max="3650" value="{{.Notifications.RetentionDays}}" required></label>
<button type="submit">Save notification preferences</button></form>
<p class="muted">Retention does not run automatically. To delete old delivered events, use the separate confirmation below. Pending alerts are always preserved.</p>
<form method="post" action="/settings/events/prune"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label class="checkline"><input type="checkbox" name="confirm" value="yes" required> Confirm deletion of expired delivered events only</label><button type="submit">Prune delivered event history</button></form>
<p><a class="text-action" href="/events">Browse and filter event history →</a></p></section>
<section class="section panel settings-panel"><h2>Legacy manual project registration</h2><p class="muted">Not required for label-based monitoring. All paths must be absolute and inside an operator-approved source root; registration never enables deployments.</p>
<form method="post" action="/settings/projects"><input type="hidden" name="csrf_token" value="{{.CSRF}}">
<label>Compose project name<input name="name" required maxlength="128" value="{{.SuggestedProject}}" placeholder="example"></label>
<label>Project directory<input name="project_dir" required placeholder="/srv/apps/example"></label>
<label>Compose files (one absolute path per line)<textarea name="files" rows="3" required placeholder="/srv/apps/example/compose.yaml"></textarea></label>
<label>Services (comma-separated)<input name="services" required value="{{.SuggestedService}}" placeholder="web, worker"></label>
<label>Environment files (one absolute path per line; optional)<textarea name="env_files" rows="2"></textarea></label>
<label>Profiles (comma-separated; optional)<input name="profiles"></label>
<button type="submit">Register for monitoring</button></form></section>
<section class="section panel settings-panel"><h2>Legacy manual watched service</h2><p class="muted">This older form configures registered projects only. Use the service controls above for automatically discovered services.</p>
<form method="post" action="/settings/policies"><input type="hidden" name="csrf_token" value="{{.CSRF}}">
<label>Project<input name="project" required value="{{.SuggestedProject}}" placeholder="example"></label>
<label>Service<input name="service" required value="{{.SuggestedService}}" placeholder="web"></label>
<label>Tagged image<input name="image" required value="{{.SuggestedImage}}" placeholder="ghcr.io/org/app:latest"></label>
<label>Platform<input name="platform" required value="linux/amd64"></label>
<label>Interval (seconds, 60–86400)<input name="interval_seconds" type="number" min="60" max="86400" value="300" required></label>
<label class="checkline"><input name="paused" type="checkbox" value="true"> Pause monitoring</label>
<button type="submit">Save monitoring policy</button></form></section>
<section class="section panel settings-panel"><h2>Registered projects</h2><div class="table-scroll"><table><thead><tr><th>Project</th><th>Services</th><th>Directory</th></tr></thead><tbody>{{range .Projects}}<tr><td>{{.Name}}</td><td>{{range .Services}}{{.}} {{end}}</td><td class="mono">{{.ProjectDir}}</td></tr>{{else}}<tr><td colspan="3" class="table-empty">None registered yet.</td></tr>{{end}}</tbody></table></div>
<h2>Existing schedules</h2><div class="table-scroll"><table><thead><tr><th>Project/service</th><th>Image</th><th>Interval</th><th>State</th></tr></thead><tbody>{{range .Policies}}<tr><td>{{.Project}} / {{.Service}}</td><td class="mono">{{.Image}}</td><td>{{.IntervalSeconds}}s</td><td>{{if .Paused}}Paused{{else}}Scheduled{{end}}</td></tr>{{else}}<tr><td colspan="4" class="table-empty">None configured yet.</td></tr>{{end}}</tbody></table></div></section>
<footer>No automatic image pulls, deployment controls, or credential storage.</footer>{{template "shell-end" .}}</body></html>`

func (h Handler) Routes(mux *http.ServeMux){
 mux.HandleFunc("GET /settings",h.index)
 mux.HandleFunc("POST /settings/monitoring/default",h.saveDefaultInterval)
 mux.HandleFunc("POST /settings/monitoring/service",h.saveServiceSettings)
 mux.HandleFunc("POST /settings/notifications",h.notifications)
 mux.HandleFunc("POST /settings/events/prune",h.prune)
 mux.HandleFunc("GET /events",h.eventsPage)
 mux.HandleFunc("POST /settings/projects",h.register)
 mux.HandleFunc("POST /settings/policies",h.policy)
}
func (h Handler) index(w http.ResponseWriter,r *http.Request){
 projects,e:=h.Backend.Projects(r.Context());if e!=nil{http.Error(w,"Settings unavailable",500);return}
 policies,e:=h.Backend.MonitorPolicies(r.Context());if e!=nil{http.Error(w,"Settings unavailable",500);return}
 discovered,e:=h.Backend.Containers(r.Context());if e!=nil{http.Error(w,"Settings unavailable",500);return}
 settings,e:=h.Backend.NotificationSettings(r.Context());if e!=nil{http.Error(w,"Settings unavailable",500);return}
 defaultInterval,e:=h.Backend.DefaultMonitorInterval(r.Context());if e!=nil{http.Error(w,"Settings unavailable",500);return}
 services,e:=h.Backend.MonitorServiceViews(r.Context());if e!=nil{http.Error(w,"Settings unavailable",500);return}
 draft:=pageData{ActivePage:"settings",MainClass:"settings-page",CSRF:h.Guard.Token(),Projects:projects,Policies:policies,Services:services,DefaultInterval:defaultInterval,Discovered:discovered,Notifications:settings,Notice:notice(r.URL.Query().Get("saved"))}
 selectedProject:=r.URL.Query().Get("project")
 selectedService:=r.URL.Query().Get("service")
  draft.SelectedProject=selectedProject;draft.SelectedService=selectedService
 for _,c:=range discovered{
  if c.Project==selectedProject&&c.Service==selectedService {
   draft.SuggestedProject=c.Project
   draft.SuggestedService=c.Service
   draft.SuggestedImage=c.Image
   break
  }
 }
 w.Header().Set("Content-Type","text/html; charset=utf-8")
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("Referrer-Policy","same-origin")
 w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'self'; img-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
 _=tmpl.Execute(w,draft)
}
func notice(code string)string{switch code{case "project":return "Project registered for monitoring.";case "policy":return "Monitoring policy saved.";case "notifications":return "Notification preferences saved.";case "pruned":return "Expired delivered events pruned.";case "default":return "Default monitoring interval saved.";case "service":return "Service monitoring settings saved.";default:return ""}}
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

func(h Handler) notifications(w http.ResponseWriter,r *http.Request){
 if !h.prepare(w,r){return}
 threshold,e:=strconv.Atoi(r.PostFormValue("outage_failures"));if e!=nil{http.Error(w,"Invalid outage threshold",400);return}
 days,e:=strconv.Atoi(r.PostFormValue("retention_days"));if e!=nil{http.Error(w,"Invalid retention",400);return}
 pref:=store.NotificationSettings{
  DigestChanges:r.PostFormValue("digest_changes")=="on",
  RegistryFailures:r.PostFormValue("registry_failures")=="on",
  RegistryRecovery:r.PostFormValue("registry_recovery")=="on",
  ProlongedOutage:r.PostFormValue("prolonged_outage")=="on",
  OutageFailures:threshold,RetentionDays:days,
 }
 if e=h.Backend.SetNotificationSettings(r.Context(),pref);e!=nil{http.Error(w,"Invalid notification settings",400);return}
 http.Redirect(w,r,"/settings?saved=notifications",http.StatusSeeOther)
}
func(h Handler) prune(w http.ResponseWriter,r *http.Request){
 if !h.prepare(w,r){return}
 if r.PostFormValue("confirm")!="yes"{http.Error(w,"Explicit pruning confirmation required",400);return}
 if _,err:=h.Backend.PruneDeliveredEvents(r.Context(),time.Now().UTC());err!=nil{http.Error(w,"Unable to prune events",500);return}
 http.Redirect(w,r,"/settings?saved=pruned",http.StatusSeeOther)
}
type eventPageData struct{
 ActivePage string
 MainClass string
 Events []store.Event
 Kind string
 DeliveryState string
 Search string
 NextBefore int64
}
var eventsHTML=template.Must(layout.WithShell(template.Must(template.New("events").Funcs(template.FuncMap{"eventKind":presentation.EventKind,"eventScope":presentation.EventScope,"eventDelivery":presentation.EventDelivery,"shortDigest":presentation.ShortDigest}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Events · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head><body>{{template "shell-start" .}}<header class="topbar"><div><p class="eyebrow">MONITORING HISTORY</p><h1>Events</h1><p class="muted">Digest movement does not confirm a newer release.</p></div><a class="text-action" href="/settings">← Settings</a></header>
<section class="section panel settings-panel"><form method="get" action="/events"><label>Event type<select name="kind"><option value="">All types</option><option value="remote_digest_changed" {{if eq .Kind "remote_digest_changed"}}selected{{end}}>Digest changed</option><option value="registry_failure" {{if eq .Kind "registry_failure"}}selected{{end}}>Registry failure</option><option value="registry_recovered" {{if eq .Kind "registry_recovered"}}selected{{end}}>Recovered</option><option value="registry_outage_prolonged" {{if eq .Kind "registry_outage_prolonged"}}selected{{end}}>Prolonged outage</option></select></label><label>Delivery status<select name="delivery"><option value="">All</option><option value="pending" {{if eq .DeliveryState "pending"}}selected{{end}}>Pending</option><option value="delivered" {{if eq .DeliveryState "delivered"}}selected{{end}}>Delivered</option></select></label><label>Search project, service or image<input name="search" maxlength="128" value="{{.Search}}"></label><button type="submit">Apply filters</button></form></section>
<p class="helper">Registry digest-change events are image/platform scoped and can affect multiple services. Webhook delivery is separate from registry monitoring: pending with zero attempts may mean no notifier is configured.</p>
<section class="section panel table-scroll"><table><thead><tr><th>ID</th><th>UTC time</th><th>Observation</th><th>Scope</th><th>Image / manifest evidence</th><th>Webhook delivery</th></tr></thead><tbody>
{{range .Events}}<tr><td>{{.ID}}</td><td>{{.OccurredAt}}</td><td><strong>{{eventKind .Kind}}</strong></td>
<td>{{eventScope .}}</td><td class="mono image-col">{{.Image}}<small class="subtext">{{.Platform}}</small>
{{if .PreviousDigest}}<small class="subtext" title="{{.PreviousDigest}}">Previous: {{shortDigest .PreviousDigest}}</small>{{end}}
{{if .Digest}}<small class="subtext" title="{{.Digest}}">Current: {{shortDigest .Digest}}</small>{{end}}</td>
<td>{{eventDelivery .}}</td></tr>
{{else}}<tr><td colspan="6" class="table-empty">No events match these filters.</td></tr>{{end}}
</tbody></table></section>
{{if .NextBefore}}<form method="get" action="/events"><input type="hidden" name="kind" value="{{.Kind}}"><input type="hidden" name="delivery" value="{{.DeliveryState}}"><input type="hidden" name="search" value="{{.Search}}"><input type="hidden" name="before" value="{{.NextBefore}}"><button type="submit">Older events →</button></form>{{end}}
<footer>No automatic Docker updates or deployments.</footer>{{template "shell-end" .}}</body></html>`))))
func(h Handler) eventsPage(w http.ResponseWriter,r *http.Request){
 q:=r.URL.Query()
 before:=int64(0)
 if q.Has("before"){v,e:=strconv.ParseInt(q.Get("before"),10,64);if e!=nil||v<=0{http.Error(w,"Invalid cursor",400);return};before=v}
 f:=store.EventFilter{Kind:q.Get("kind"),DeliveryState:q.Get("delivery"),Search:q.Get("search"),BeforeID:before,Limit:50}
 events,e:=h.Backend.FilterEvents(r.Context(),f);if e!=nil{http.Error(w,"Invalid event filter",400);return}
 data:=eventPageData{ActivePage:"events",MainClass:"settings-page",Events:events,Kind:f.Kind,DeliveryState:f.DeliveryState,Search:f.Search}
 if len(events)==50{data.NextBefore=events[len(events)-1].ID}
 w.Header().Set("Content-Type","text/html; charset=utf-8")
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
 _=eventsHTML.Execute(w,data)
}
