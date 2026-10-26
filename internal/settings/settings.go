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
 "github.com/docklantern/docklantern/internal/poc"
 "github.com/docklantern/docklantern/internal/store"
)

type Backend interface {
 Projects(context.Context)([]store.Registration,error)
 MonitorPolicies(context.Context)([]store.MonitorPolicy,error)
 RegistryChecks(context.Context)([]store.RegistryCheck,error)
 Containers(context.Context)([]store.Container,error)
 Register(context.Context,store.Registration)error
 SetMonitorPolicy(context.Context,store.MonitorPolicy)error
 NotificationSettings(context.Context)(store.NotificationSettings,error)
 SetNotificationSettings(context.Context,store.NotificationSettings)error
 PruneDeliveredEvents(context.Context,time.Time)(int64,error)
 FilterEvents(context.Context,store.EventFilter)([]store.Event,error)
 LatestImageChangeEvents(context.Context)([]store.Event,error)
 ImageEvents(context.Context,string,string,int64,int)([]store.Event,error)
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
const pageHTML=`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="color-scheme" content="dark"><title>Settings · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head>
<body>{{template "shell-start" .}}<header class="topbar"><div><p class="eyebrow">DOCKER COMPOSE / SETTINGS</p><h1>Settings</h1><p class="muted">Monitoring settings for the whole application. Manage individual services from the <a href="/services">Services workspace</a>.</p></div><span class="readonly">MONITORING ONLY</span></header>
{{if .Notice}}<p role="status" class="notice">{{.Notice}}</p>{{end}}
<section class="section panel settings-panel" id="monitoring-defaults">
<h2>Monitoring defaults</h2>
<p class="muted">New label- or GUI-enrolled services inherit this interval unless they have a Docker interval label or GUI override. Changes apply to inheriting watches without restarting a container.</p>
<form method="post" action="/settings/monitoring/default">
<input type="hidden" name="csrf_token" value="{{.CSRF}}">
<label>Default check interval (seconds, 60–86400)
<input type="number" list="monitoring-interval-presets" name="interval_seconds" min="60" max="86400" value="{{.DefaultInterval}}" required></label>
<datalist id="monitoring-interval-presets"><option value="60" label="1 minute"></option><option value="300" label="5 minutes"></option><option value="600" label="10 minutes"></option><option value="900" label="15 minutes"></option><option value="1800" label="30 minutes"></option><option value="3600" label="1 hour"></option><option value="21600" label="6 hours"></option><option value="86400" label="24 hours"></option></datalist><button type="submit">Save global default</button>
</form><p class="helper">Choose a suggested interval or enter a custom number of seconds. Per-service overrides are managed on <a href="/services">Service Details</a>.</p></section>
<section class="section panel settings-panel"><h2>Notification preferences</h2><p class="muted">The webhook sender must be configured separately using an HTTPS URL and a signing key file. No secrets are stored here. Muted events remain in history and pending delivery until re-enabled.</p>
<form method="post" action="/settings/notifications"><input type="hidden" name="csrf_token" value="{{.CSRF}}">
<label class="checkline"><input type="checkbox" name="digest_changes" value="on" {{if .Notifications.DigestChanges}}checked{{end}}> Remote digest changed</label>
<label class="checkline"><input type="checkbox" name="registry_failures" value="on" {{if .Notifications.RegistryFailures}}checked{{end}}> Registry check failed</label>
<label class="checkline"><input type="checkbox" name="registry_recovery" value="on" {{if .Notifications.RegistryRecovery}}checked{{end}}> Registry check recovered</label>
<label class="checkline"><input type="checkbox" name="prolonged_outage" value="on" {{if .Notifications.ProlongedOutage}}checked{{end}}> Prolonged registry outage</label>
<label>Consecutive failures to alert (2–100)<input type="number" name="outage_failures" min="2" max="100" value="{{.Notifications.OutageFailures}}" required></label>

<button type="submit">Save notification preferences</button></form>


<p class="helper">Webhook delivery requires a separately configured HTTPS notifier and signing key. Settings here do not create a notifier.</p></section>
<section class="section panel settings-panel" id="history-retention"><h2>History and retention</h2><p class="muted">Choose how long delivered monitoring events are eligible for manual cleanup. Retention does not prune events automatically. Pending alerts are always preserved.</p>
<form method="post" action="/settings/retention"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label>Event retention days (1–3650)<input type="number" name="retention_days" min="1" max="3650" value="{{.Notifications.RetentionDays}}" required></label><button type="submit">Save retention period</button></form>
<div class="settings-danger-zone"><h3>Prune delivered history</h3><p class="helper">Deletion requires explicit confirmation and affects expired delivered events only.</p><form method="post" action="/settings/events/prune"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label class="checkline"><input type="checkbox" name="confirm" value="yes" required> Confirm deletion of expired delivered events only</label><button type="submit">Prune delivered event history</button></form></div>
<p><a class="text-action" href="/events">Browse and filter event history</a></p></section>
<details class="settings-advanced" id="advanced" {{if .SelectedProject}}open{{end}}><summary>Advanced / Legacy Configuration</summary>
<p class="helper">Optional manual registrations and older service controls. Normal per-service monitoring is managed under <a href="/services">Services</a>.</p>
<details class="settings-subsection" id="service-monitoring" {{if .SelectedProject}}open{{end}}><summary>Service monitoring (legacy controls)</summary><section class="section panel settings-panel" id="service-monitoring">
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
</section></details>
<section class="section panel settings-panel"><h2>Discovered services</h2><p class="muted">Discovered containers are also listed in the monitoring controls above. Prefill the legacy source registration form below only if you need to retain an existing manually configured project.</p>
<div class="table-scroll"><table><thead><tr><th>Project</th><th>Service</th><th>Image</th><th>Choose</th></tr></thead><tbody>
{{range .Discovered}}<tr><td>{{.Project}}</td><td>{{.Service}}</td><td class="mono">{{.Image}}</td><td><form method="get" action="/settings"><input type="hidden" name="project" value="{{.Project}}"><input type="hidden" name="service" value="{{.Service}}"><button type="submit">Prefill</button></form></td></tr>
{{else}}<tr><td colspan="4" class="table-empty">No services discovered. Enable the optional discovery proxy and inventory scanning to populate this list.</td></tr>{{end}}
</tbody></table></div></section><section class="section panel settings-panel"><h2>Legacy manual project registration</h2><p class="muted">Not required for label-based monitoring. All paths must be absolute and inside an operator-approved source root; registration never enables deployments.</p>
<form method="post" action="/settings/projects"><input type="hidden" name="csrf_token" value="{{.CSRF}}">
<label>Compose project name<input name="name" required maxlength="128" value="{{.SuggestedProject}}" placeholder="example"></label>
<label>Project directory<input name="project_dir" required placeholder="/srv/apps/example"></label>
<label>Compose files (one absolute path per line)<textarea name="files" rows="3" required placeholder="/srv/apps/example/compose.yaml"></textarea></label>
<label>Services (comma-separated)<input name="services" required value="{{.SuggestedService}}" placeholder="web, worker"></label>
<label>Environment files (one absolute path per line; optional)<textarea name="env_files" rows="2"></textarea></label>
<label>Profiles (comma-separated; optional)<input name="profiles"></label>
<button type="submit">Register for monitoring</button></form></section><section class="section panel settings-panel"><h2>Legacy manual watched service</h2><p class="muted">This older form configures registered projects only. Use the service controls above for automatically discovered services.</p>
<form method="post" action="/settings/policies"><input type="hidden" name="csrf_token" value="{{.CSRF}}">
<label>Project<input name="project" required value="{{.SuggestedProject}}" placeholder="example"></label>
<label>Service<input name="service" required value="{{.SuggestedService}}" placeholder="web"></label>
<label>Tagged image<input name="image" required value="{{.SuggestedImage}}" placeholder="ghcr.io/org/app:latest"></label>
<label>Platform<input name="platform" required value="linux/amd64"></label>
<label>Interval (seconds, 60–86400)<input name="interval_seconds" type="number" min="60" max="86400" value="300" required></label>
<label class="checkline"><input name="paused" type="checkbox" value="true"> Pause monitoring</label>
<button type="submit">Save monitoring policy</button></form></section><section class="section panel settings-panel"><h2>Registered projects</h2><div class="table-scroll"><table><thead><tr><th>Project</th><th>Services</th><th>Directory</th></tr></thead><tbody>{{range .Projects}}<tr><td>{{.Name}}</td><td>{{range .Services}}{{.}} {{end}}</td><td class="mono">{{.ProjectDir}}</td></tr>{{else}}<tr><td colspan="3" class="table-empty">None registered yet.</td></tr>{{end}}</tbody></table></div>
<h2>Existing schedules</h2><div class="table-scroll"><table><thead><tr><th>Project/service</th><th>Image</th><th>Interval</th><th>State</th></tr></thead><tbody>{{range .Policies}}<tr><td>{{.Project}} / {{.Service}}</td><td class="mono">{{.Image}}</td><td>{{.IntervalSeconds}}s</td><td>{{if .Paused}}Paused{{else}}Scheduled{{end}}</td></tr>{{else}}<tr><td colspan="4" class="table-empty">None configured yet.</td></tr>{{end}}</tbody></table></div></section>
</details>
<footer>No automatic image pulls, deployment controls, or credential storage.</footer>{{template "shell-end" .}}</body></html>`

func (h Handler) Routes(mux *http.ServeMux){
 mux.HandleFunc("GET /images",h.imagesPage)
 mux.HandleFunc("GET /images/{id}",h.imageDetailsPage)
 mux.HandleFunc("GET /services",h.servicesPage)
 mux.HandleFunc("GET /services/{project}/{service}",h.serviceDetailsPage)
 mux.HandleFunc("GET /settings",h.index)
 mux.HandleFunc("POST /settings/monitoring/default",h.saveDefaultInterval)
 mux.HandleFunc("POST /settings/monitoring/service",h.saveServiceSettings)
 mux.HandleFunc("POST /settings/notifications",h.notifications)
 mux.HandleFunc("POST /settings/retention",h.saveRetention)
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
func notice(code string)string{switch code{case "project":return "Project registered for monitoring.";case "policy":return "Monitoring policy saved.";case "notifications":return "Notification preferences saved.";case "pruned":return "Expired delivered events pruned.";case "default":return "Default monitoring interval saved.";case "service":return "Service monitoring settings saved.";case "retention":return "History retention period saved.";default:return ""}}
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
 // Notification preference edits must not overwrite a separately saved
 // retention period from an older browser tab. Use persisted retention.
 current,e:=h.Backend.NotificationSettings(r.Context())
 if e!=nil{http.Error(w,"Notification settings unavailable",http.StatusInternalServerError);return}
 pref:=store.NotificationSettings{
  DigestChanges:r.PostFormValue("digest_changes")=="on",
  RegistryFailures:r.PostFormValue("registry_failures")=="on",
  RegistryRecovery:r.PostFormValue("registry_recovery")=="on",
  ProlongedOutage:r.PostFormValue("prolonged_outage")=="on",
  OutageFailures:threshold,RetentionDays:current.RetentionDays,
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

// saveRetention preserves existing notifier preferences, using the same
// CSRF/origin checks and validated settings store as existing forms.
func(h Handler) saveRetention(w http.ResponseWriter,r *http.Request){
 if !h.prepare(w,r){return}
 days,err:=strconv.Atoi(r.PostFormValue("retention_days"))
 if err!=nil||days<1||days>3650{http.Error(w,"Invalid retention",http.StatusBadRequest);return}
 current,err:=h.Backend.NotificationSettings(r.Context())
 if err!=nil{http.Error(w,"Retention unavailable",http.StatusInternalServerError);return}
 current.RetentionDays=days
 if err=h.Backend.SetNotificationSettings(r.Context(),current);err!=nil{
  http.Error(w,"Unable to save retention",http.StatusBadRequest);return
 }
 http.Redirect(w,r,"/settings?saved=retention#history-retention",http.StatusSeeOther)
}
