package settings

import (
 "context"
 "html/template"
 "net/http"
 "net/url"
 "sort"
 "time"

 "github.com/docklantern/docklantern/internal/layout"
 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/store"
 "github.com/docklantern/docklantern/internal/updates"
)

// Workspace routes are read-only. Service controls remain in the existing,
// authenticated/CSRF-protected Settings POST handlers until the service-detail
// management experience is implemented in beta.4 phase 4.
type workspaceData struct {
 ActivePage string
 MainClass string
 Images []presentation.ImageSummary
 Services []presentation.ServiceSummary
 Groups []serviceGroup
 Image *presentation.ImageSummary
 Service *presentation.ServiceSummary
 Associated []presentation.ServiceSummary
 SettingsURL string
}

type serviceGroup struct {
 Project string
 Services []presentation.ServiceSummary
}

var workspaceFuncs = template.FuncMap{
 "serviceURL":func(project,service string)string{
  return "/services/"+url.PathEscape(project)+"/"+url.PathEscape(service)
 },
 "imageURL":func(id string)string{return "/images/"+url.PathEscape(id)},
 "status":func(s string)string{
  switch s {
  case "check_failed":return "Check failed"
  case "not_checked":return "Not checked"
  case "first_seen":return "First observed"
  case "changed":return "Remote changed"
  case "unchanged":return "Unchanged"
  case "stale":return "Stale observation"
  case "not_running":return "Not running"
  case "not_enrolled":return "Not enrolled"
  case "disabled_by_label":return "Disabled by label"
  case "invalid_discovery":return "Invalid discovery"
  case "not_scheduled":return "Not scheduled"
  case "monitoring":return "Monitoring"
  case "paused":return "Paused"
  case "current":return "Current"
  case "different":return "Different"
  case "mixed":return "Mixed"
  default:return "Unknown"
  }
 },
 "when":func(s string)string{
  if s==""{return "Not yet"}
  at,err:=time.Parse(time.RFC3339Nano,s)
  if err!=nil{return "Unknown"}
  return at.UTC().Format("2006-01-02 15:04 UTC")
 },
}

func workspaceTemplate(name,markup string)*template.Template {
 return template.Must(layout.WithShell(template.Must(template.New(name).Funcs(workspaceFuncs).Parse(markup))))
}

var imagesTemplate=workspaceTemplate("images",`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>Images · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head><body>
{{template "shell-start" .}}
<header class="topbar"><div><p class="eyebrow">DOCKER COMPOSE / IMAGES</p><h1>Images</h1><p class="muted">Watched registry images and deployed-image evidence.</p></div><span class="readonly">MONITORING ONLY</span></header>
<section class="section" aria-labelledby="watched-images-title"><div class="section-head"><div><h2 id="watched-images-title">Watched images</h2><p class="helper">One entry per image reference and platform. A remote digest change does not prove a newer version is available.</p></div></div>
<div class="panel table-scroll"><table><thead><tr><th>Image</th><th>Platform</th><th>Registry</th><th>Deployed</th><th>Services</th><th>Details</th></tr></thead><tbody>
{{range .Images}}<tr><td class="mono image-col">{{.Reference}}</td><td>{{.Platform}}</td><td>{{status .RegistryStatus}}</td><td>{{status .DeploymentStatus}}</td><td>{{len .Services}}</td><td><a class="text-action" href="{{imageURL .ID}}">View image</a></td></tr>
{{else}}<tr><td colspan="6" class="table-empty">No watched images yet. Enable monitoring on a discovered Compose service to begin.</td></tr>{{end}}
</tbody></table></div></section>
<footer>No automatic image pulls or deployments.</footer>{{template "shell-end" .}}</body></html>`)

var imageDetailTemplate=workspaceTemplate("image-detail",`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>Image Details · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head><body>
{{template "shell-start" .}}
<nav class="breadcrumbs" aria-label="Breadcrumb"><a href="/images">Images</a><span aria-hidden="true">/</span><span aria-current="page">Image Details</span></nav>
<header class="topbar"><div><p class="eyebrow">IMAGE DETAILS</p><h1 class="workspace-title mono">{{.Image.Reference}}</h1><p class="muted">{{.Image.Platform}}</p></div><span class="readonly">MONITORING ONLY</span></header>
<section class="section"><div class="workspace-stats"><article class="stat-card"><div class="stat-label">Registry observation</div><strong class="workspace-value">{{status .Image.RegistryStatus}}</strong><span class="stat-hint">{{with .Image.LatestObservation}}Last successful check: {{when .CheckedAt}}{{else}}Not checked yet{{end}}</span></article>
<article class="stat-card"><div class="stat-label">Deployment assessment</div><strong class="workspace-value">{{status .Image.DeploymentStatus}}</strong><span class="stat-hint">Current: {{.Image.DeploymentCounts.Current}} · Different: {{.Image.DeploymentCounts.Different}} · Unknown: {{.Image.DeploymentCounts.Unknown}}</span></article>
<article class="stat-card"><div class="stat-label">Watched by</div><strong class="workspace-value">{{len .Image.Services}} services</strong><span class="stat-hint">{{if .Image.LastChangeAt}}Last recorded digest change: {{when .Image.LastChangeAt}}{{else}}No change found in recent events{{end}}</span></article></div></section>
<section class="section" aria-labelledby="image-services-title"><h2 id="image-services-title">Associated services</h2><p class="helper">Each Compose service has independent monitoring settings.</p><div class="panel table-scroll"><table><thead><tr><th>Compose service</th><th>Monitoring</th><th>Deployment</th><th>Inspect</th></tr></thead><tbody>
{{range .Associated}}<tr><td>{{.Project}} / {{.Service}}</td><td>{{status .Monitoring}}</td><td>{{status .Assessment}}</td><td><a class="text-action" href="{{serviceURL .Project .Service}}">View service</a></td></tr>
{{else}}<tr><td colspan="4" class="table-empty">No associated discovered service details available.</td></tr>{{end}}</tbody></table></div></section>
<p class="helper">Registry checks and deployed digest comparisons are independent observations, not release-version judgments.</p>
<footer>No Docker mutation or update execution.</footer>{{template "shell-end" .}}</body></html>`)

var servicesTemplate=workspaceTemplate("services",`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>Services · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head><body>
{{template "shell-start" .}}
<header class="topbar"><div><p class="eyebrow">DOCKER COMPOSE / SERVICES</p><h1>Services</h1><p class="muted">Discovered Compose services, monitoring, and deployment context.</p></div><span class="readonly">MONITORING ONLY</span></header>
{{range .Groups}}<section class="section" aria-label="Compose project {{.Project}}"><div class="section-head"><h2>{{.Project}}</h2><span class="helper">{{len .Services}} services</span></div>
<div class="panel table-scroll"><table><thead><tr><th>Service</th><th>Image</th><th>Monitoring</th><th>Containers</th><th>Details</th></tr></thead><tbody>
{{range .Services}}<tr><td>{{.Service}}</td><td class="mono image-col">{{.Image}}</td><td>{{status .Monitoring}}</td><td>{{.RunningCount}} / {{.ContainerCount}} running</td><td><a class="text-action" href="{{serviceURL .Project .Service}}">View service</a></td></tr>{{end}}
</tbody></table></div></section>
{{else}}<section class="section panel empty"><h2>No Compose services discovered</h2><p>Enable Docker discovery to populate this workspace; monitored and unmonitored services will both appear.</p></section>{{end}}
<footer>Monitoring enrollment does not grant deployment permissions.</footer>{{template "shell-end" .}}</body></html>`)

var serviceDetailTemplate=workspaceTemplate("service-detail",`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>Service Details · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head><body>
{{template "shell-start" .}}
<nav class="breadcrumbs" aria-label="Breadcrumb"><a href="/services">Services</a><span aria-hidden="true">/</span><span aria-current="page">{{.Service.Project}} / {{.Service.Service}}</span></nav>
<header class="topbar"><div><p class="eyebrow">SERVICE DETAILS</p><h1>{{.Service.Project}} / {{.Service.Service}}</h1><p class="muted">Compose deployment and monitoring configuration.</p></div><span class="readonly">MONITORING ONLY</span></header>
<section class="section"><div class="workspace-stats">
<article class="stat-card"><div class="stat-label">Monitoring</div><strong class="workspace-value">{{status .Service.Monitoring}}</strong><span class="stat-hint">{{.Service.Enrollment}}</span></article>
<article class="stat-card"><div class="stat-label">Effective interval</div><strong class="workspace-value">{{if .Service.HasPolicy}}{{.Service.IntervalSeconds}} seconds{{else}}Not scheduled{{end}}</strong><span class="stat-hint">{{.Service.IntervalSource}}</span></article>
<article class="stat-card"><div class="stat-label">Running containers</div><strong class="workspace-value">{{.Service.RunningCount}} / {{.Service.ContainerCount}}</strong><span class="stat-hint">Docker health: {{.Service.ContainerHealth}}</span></article>
</div></section>
<section class="section panel workspace-detail"><h2>Image and deployment</h2><p class="mono">{{.Service.Image}}</p><p class="muted">Platform: {{.Service.Platform}} · Deployment assessment: {{status .Service.Assessment}}</p>
{{if .Service.ImageID}}<p><a class="text-action" href="{{imageURL .Service.ImageID}}">View image details</a></p>{{end}}</section>
<section class="section panel workspace-detail"><h2>Monitoring configuration</h2><p class="muted">Changes continue to use the existing authenticated and CSRF-protected service settings forms.</p>
<p><a class="text-action" href="{{.SettingsURL}}">Manage monitoring settings</a></p></section>
<footer>Service monitoring controls do not authorize Docker deployments.</footer>{{template "shell-end" .}}</body></html>`)

// All workspace views use the existing store and OCI assessment code; these
// GET handlers never contact the registry, write SQLite, or execute Docker.
func(h Handler) workspaceSnapshot(ctx context.Context)(workspaceData,error){
 projects,err:=h.Backend.Projects(ctx);if err!=nil{return workspaceData{},err}
 containers,err:=h.Backend.Containers(ctx);if err!=nil{return workspaceData{},err}
 policies,err:=h.Backend.MonitorPolicies(ctx);if err!=nil{return workspaceData{},err}
 checks,err:=h.Backend.RegistryChecks(ctx);if err!=nil{return workspaceData{},err}
 views,err:=h.Backend.MonitorServiceViews(ctx);if err!=nil{return workspaceData{},err}
 events,err:=h.Backend.FilterEvents(ctx,store.EventFilter{Kind:"remote_digest_changed",Limit:200})
 if err!=nil{return workspaceData{},err}
 now:=time.Now().UTC()
 assessments:=updates.AssessMonitoredFresh(projects,containers,policies,checks,now)
 result:=workspaceData{
  Images:presentation.BuildImageSummaries(policies,checks,assessments,events,now),
  Services:presentation.BuildServiceSummaries(views,containers,policies,assessments),
 }
 for _,s:=range result.Services{
  if len(result.Groups)==0 || result.Groups[len(result.Groups)-1].Project!=s.Project{
   result.Groups=append(result.Groups,serviceGroup{Project:s.Project})
  }
  last:=len(result.Groups)-1
  result.Groups[last].Services=append(result.Groups[last].Services,s)
 }
 return result,nil
}

func workspaceHeaders(w http.ResponseWriter) {
 w.Header().Set("Content-Type","text/html; charset=utf-8")
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("Referrer-Policy","no-referrer")
 w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'self'; img-src 'self'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'")
}
func renderWorkspace(w http.ResponseWriter,t *template.Template,data workspaceData){
 workspaceHeaders(w)
 _=t.Execute(w,data)
}
func(h Handler) imagesPage(w http.ResponseWriter,r *http.Request){
 d,err:=h.workspaceSnapshot(r.Context());if err!=nil{http.Error(w,"Images unavailable",500);return}
 d.ActivePage="images"
 renderWorkspace(w,imagesTemplate,d)
}
func(h Handler) imageDetailsPage(w http.ResponseWriter,r *http.Request){
 id,err:=presentation.ParseImageID(r.PathValue("id"))
 if err!=nil{http.NotFound(w,r);return}
 d,err:=h.workspaceSnapshot(r.Context());if err!=nil{http.Error(w,"Image unavailable",500);return}
 for i:=range d.Images{
  item:=&d.Images[i]
  if item.Reference!=id.Reference || item.Platform!=id.Platform{continue}
  d.Image=item
  watchers:=make(map[presentation.ServiceIdentity]struct{},len(item.Services))
  for _,s:=range item.Services{watchers[s]=struct{}{}}
  for _,s:=range d.Services{
   if _,ok:=watchers[s.ServiceIdentity];ok{d.Associated=append(d.Associated,s)}
  }
  sort.Slice(d.Associated,func(i,j int)bool{
   a,b:=d.Associated[i],d.Associated[j]
   if a.Project!=b.Project{return a.Project<b.Project}
   return a.Service<b.Service
  })
  d.ActivePage="images"
  renderWorkspace(w,imageDetailTemplate,d)
  return
 }
 http.NotFound(w,r)
}
func(h Handler) servicesPage(w http.ResponseWriter,r *http.Request){
 d,err:=h.workspaceSnapshot(r.Context());if err!=nil{http.Error(w,"Services unavailable",500);return}
 d.ActivePage="services"
 renderWorkspace(w,servicesTemplate,d)
}
func(h Handler) serviceDetailsPage(w http.ResponseWriter,r *http.Request){
 project,service:=r.PathValue("project"),r.PathValue("service")
 if project=="" || service=="" || len(project)>128 || len(service)>128{http.NotFound(w,r);return}
 d,err:=h.workspaceSnapshot(r.Context());if err!=nil{http.Error(w,"Service unavailable",500);return}
 for i:=range d.Services{
  if d.Services[i].Project!=project || d.Services[i].Service!=service {continue}
  d.Service=&d.Services[i]
  d.SettingsURL="/settings?project="+url.QueryEscape(project)+"&service="+url.QueryEscape(service)+"#service-monitoring"
  d.ActivePage="services"
  renderWorkspace(w,serviceDetailTemplate,d)
  return
 }
 http.NotFound(w,r)
}

