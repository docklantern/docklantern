package settings

import (
 "errors"
 "net/http"
 "net/url"
 "sort"
 "strings"

 "github.com/docklantern/docklantern/internal/presentation"
)

type serviceFilters struct {
 Query string
 Monitoring string
}

type serviceListData struct {
 workspaceData
 Filter serviceFilters
 Total int
 Visible int
 Enrolled int
 NotEnrolled int
 Attention int
}

type serviceDetailsData struct {
 workspaceData
 CSRF string
 ReturnTo string
 Saved bool
 CanOverride bool
 CanReset bool
 Reason string
 RemoteDigest string
 DeployedDigest string
 InventoryObservedAt string
 RegistryCheckedAt string
}

func parseServiceFilters(r *http.Request)(serviceFilters,error){
 f:=serviceFilters{
  Query:strings.TrimSpace(r.URL.Query().Get("q")),
  Monitoring:r.URL.Query().Get("monitoring"),
 }
 if len(f.Query)>128 || strings.ContainsAny(f.Query,"\x00\r\n"){return serviceFilters{},errors.New("invalid service search")}
 switch f.Monitoring{
 case "","active","paused","not_enrolled","disabled_by_label","needs_attention":
 default:return serviceFilters{},errors.New("invalid monitoring filter")
 }
 return f,nil
}

func serviceNeedsAttention(s presentation.ServiceSummary)bool{
 return s.Monitoring=="check_failed" || s.Monitoring=="invalid_discovery" || s.Assessment=="different" || s.LastError!=""
}

func serviceEnrolled(s presentation.ServiceSummary)bool{
 return s.HasPolicy && s.Monitoring!="not_enrolled" && s.Monitoring!="disabled_by_label" && s.Monitoring!="invalid_discovery" && s.Enrollment!="Not monitored"
}

func filterServices(all []presentation.ServiceSummary,filter serviceFilters)[]presentation.ServiceSummary{
 out:=make([]presentation.ServiceSummary,0,len(all))
 query:=strings.ToLower(filter.Query)
 for _,s:=range all{
  if query!="" && !strings.Contains(strings.ToLower(s.Project+" "+s.Service+" "+s.Image+" "+s.Platform),query){continue}
  switch filter.Monitoring{
  case "active":
   if s.Monitoring!="monitoring" && s.Monitoring!="check_failed"{continue}
  case "paused","not_enrolled","disabled_by_label":
   if s.Monitoring!=filter.Monitoring{continue}
  case "needs_attention":
   if !serviceNeedsAttention(s){continue}
  }
  out=append(out,s)
 }
 sort.Slice(out,func(i,j int)bool{
  if out[i].Project!=out[j].Project{return out[i].Project<out[j].Project}
  return out[i].Service<out[j].Service
 })
 return out
}

func groupServices(services []presentation.ServiceSummary)[]serviceGroup{
 groups:=[]serviceGroup{}
 for _,s:=range services{
  if len(groups)==0 || groups[len(groups)-1].Project!=s.Project{
   groups=append(groups,serviceGroup{Project:s.Project})
  }
  i:=len(groups)-1
  groups[i].Services=append(groups[i].Services,s)
 }
 return groups
}

func(h Handler) renderServicesWorkspace(w http.ResponseWriter,r *http.Request){
 f,err:=parseServiceFilters(r)
 if err!=nil{http.Error(w,"Invalid services filter",http.StatusBadRequest);return}
 data,err:=h.workspaceSnapshot(r.Context())
 if err!=nil{http.Error(w,"Services unavailable",http.StatusInternalServerError);return}
 out:=serviceListData{workspaceData:data,Filter:f,Total:len(data.Services)}
 for _,s:=range data.Services{
  if serviceEnrolled(s){out.Enrolled++}
  if s.Monitoring=="not_enrolled"{out.NotEnrolled++}
  if serviceNeedsAttention(s){out.Attention++}
 }
 out.Services=filterServices(data.Services,f)
 out.Visible=len(out.Services)
 out.Groups=groupServices(out.Services)
 out.ActivePage="services"
 workspaceHeaders(w)
 _=servicesExperienceTemplate.Execute(w,out)
}

func(h Handler) renderServiceWorkspaceDetails(w http.ResponseWriter,r *http.Request){
 project,service:=r.PathValue("project"),r.PathValue("service")
 if project==""||service==""||len(project)>128||len(service)>128{
  http.NotFound(w,r);return
 }
 if saved:=r.URL.Query().Get("saved");saved!="" && saved!="service"{
  http.Error(w,"Invalid service status",http.StatusBadRequest);return
 }
 data,err:=h.workspaceSnapshot(r.Context())
 if err!=nil{http.Error(w,"Service unavailable",http.StatusInternalServerError);return}
 for i:=range data.Services{
  s:=&data.Services[i]
  if s.Project!=project || s.Service!=service{continue}
  data.Service=s
  data.ActivePage="services"
  data.SettingsURL="/settings?project="+url.QueryEscape(project)+"&service="+url.QueryEscape(service)+"#service-monitoring"
  details:=serviceDetailsData{
   workspaceData:data,
   CSRF:h.Guard.Token(),
   ReturnTo:"/services/"+url.PathEscape(project)+"/"+url.PathEscape(service),
   Saved:r.URL.Query().Get("saved")=="service",
   CanOverride:s.CanEditInterval && s.Monitoring!="disabled_by_label" && s.Monitoring!="invalid_discovery",
   CanReset:s.GUIOverride!=nil && s.CanEditInterval && s.Monitoring!="disabled_by_label" && s.Monitoring!="invalid_discovery",
  }
  for _,a:=range data.Assessments{
   if a.Project==project && a.Service==service && a.Image==s.Image && a.Platform==s.Platform{
    details.Reason=a.Reason
    details.DeployedDigest=a.DeployedDigest
    details.RemoteDigest=a.RemoteDigest
    details.InventoryObservedAt=a.InventoryObservedAt
    details.RegistryCheckedAt=a.RegistryCheckedAt
    break
   }
  }
  workspaceHeaders(w)
  _=serviceExperienceDetailTemplate.Execute(w,details)
  return
 }
 http.NotFound(w,r)
}

var servicesExperienceTemplate=workspaceTemplate("services-v2",`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>Services · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head><body>
{{template "shell-start" .}}
<header class="topbar"><div><p class="eyebrow">DOCKER COMPOSE / SERVICES</p><h1>Services</h1><p class="muted">Monitor discovered Compose services and inspect running image evidence.</p></div><span class="readonly">MONITORING ONLY</span></header>
<section class="section" aria-label="Service summary"><div class="stats service-stats">
<article class="stat-card"><div class="stat-label">Discovered and registered</div><strong>{{.Total}}</strong><span class="stat-hint">One entry per Compose project and service</span></article>
<article class="stat-card"><div class="stat-label">Enrolled services</div><strong>{{.Enrolled}}</strong><span class="stat-hint">Includes paused monitoring policies</span></article>
<article class="stat-card"><div class="stat-label">Not enrolled</div><strong>{{.NotEnrolled}}</strong><span class="stat-hint">Eligible services may opt in from details</span></article>
<article class="stat-card"><div class="stat-label">Needs attention</div><strong>{{.Attention}}</strong><span class="stat-hint">Check failures, discovery errors or differing image evidence</span></article>
</div></section>
<section class="section" aria-labelledby="compose-service-title"><div class="section-head"><div><h2 id="compose-service-title">Compose services</h2><p class="helper">Services are grouped by Compose project; replicas stay together. Monitoring and deployment evidence are separate.</p></div><span class="helper">{{.Visible}} of {{.Total}} services</span></div>
<form class="service-filters panel" action="/services" method="get" role="search">
<label>Search project, service or image<input type="search" name="q" maxlength="128" value="{{.Filter.Query}}" placeholder="project, nginx, worker..."></label>
<label>Monitoring filter<select name="monitoring">
<option value="">All services</option>
<option value="active" {{if eq .Filter.Monitoring "active"}}selected{{end}}>Active monitoring</option>
<option value="paused" {{if eq .Filter.Monitoring "paused"}}selected{{end}}>Paused</option>
<option value="not_enrolled" {{if eq .Filter.Monitoring "not_enrolled"}}selected{{end}}>Not enrolled</option>
<option value="disabled_by_label" {{if eq .Filter.Monitoring "disabled_by_label"}}selected{{end}}>Disabled by Compose label</option>
<option value="needs_attention" {{if eq .Filter.Monitoring "needs_attention"}}selected{{end}}>Needs attention</option>
</select></label>
<div class="service-filter-actions"><button type="submit">Apply filters</button><a class="text-action" href="/services">Clear</a></div>
</form>
{{range .Groups}}
<details class="service-project" open><summary><span class="project-heading">{{.Project}}</span><span class="helper">{{len .Services}} services</span></summary>
<div class="table-scroll"><table><thead><tr><th>Service</th><th>Docker-observed image / platform</th><th>Monitoring</th><th>Deployed image</th><th>Runtime</th><th>Schedule</th><th>Details</th></tr></thead><tbody>
{{range .Services}}<tr>
<td><strong>{{.Service}}</strong></td><td class="mono image-col">{{if .MixedDiscoveredImages}}Multiple container image references{{else if .DiscoveredImage}}{{.DiscoveredImage}}{{else}}No recent Docker inventory{{end}}<small class="subtext">{{.Platform}}</small>{{if .HasPolicy}}{{if ne .Image .DiscoveredImage}}<small class="subtext">Watched: {{.Image}}</small>{{end}}{{end}}</td>
<td><span class="pill {{tone .Monitoring}}">{{status .Monitoring}}</span><small class="subtext">{{.Enrollment}}</small></td>
<td><span class="pill {{tone .Assessment}}">{{status .Assessment}}</span></td>
<td>{{.RunningCount}} / {{.ContainerCount}} running<small class="subtext">Health: {{.ContainerHealth}}</small></td>
<td>{{if .HasPolicy}}{{.IntervalSeconds}}s<small class="subtext">{{.IntervalSource}}</small>{{if .NextCheckAt}}<small class="subtext">Next: {{when .NextCheckAt}}</small>{{end}}{{else}}Not scheduled{{end}}</td>
<td><a class="text-action" href="{{serviceURL .Project .Service}}">View service</a></td></tr>{{end}}
</tbody></table></div></details>
{{else}}
<section class="panel empty">{{if .Total}}<h3>No services match the selected filters</h3><p><a href="/services">Clear filters</a> to view all discovered services.</p>
{{else}}<h3>No Compose services discovered</h3><p>Configure Docker discovery, then enroll eligible services from their details page.</p>{{end}}</section>
{{end}}
</section>
<footer>Service monitoring is read-only and does not authorize Docker deployment operations.</footer>{{template "shell-end" .}}</body></html>`)

var serviceExperienceDetailTemplate=workspaceTemplate("service-detail-v2",`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>Service Details · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head><body>
{{template "shell-start" .}}
<nav class="breadcrumbs" aria-label="Breadcrumb"><a href="/services">Services</a><span aria-hidden="true">/</span><span aria-current="page">{{.Service.Project}} / {{.Service.Service}}</span></nav>
<header class="topbar"><div><p class="eyebrow">SERVICE DETAILS</p><h1>{{.Service.Project}} / {{.Service.Service}}</h1><p class="muted">Compose deployment evidence and service-level monitoring.</p></div><span class="readonly">MONITORING ONLY</span></header>
{{if .Saved}}<p class="notice" role="status">Service monitoring settings saved.</p>{{end}}
<section class="section" aria-label="Service monitoring summary"><div class="workspace-stats">
<article class="stat-card"><div class="stat-label">Monitoring</div><strong class="workspace-value"><span class="pill {{tone .Service.Monitoring}}">{{status .Service.Monitoring}}</span></strong><span class="stat-hint">{{.Service.Enrollment}}</span></article>
<article class="stat-card"><div class="stat-label">Effective interval</div><strong class="workspace-value">{{if .Service.HasPolicy}}{{.Service.IntervalSeconds}} seconds{{else}}Not scheduled{{end}}</strong><span class="stat-hint">{{.Service.IntervalSource}}</span></article>
<article class="stat-card"><div class="stat-label">Running containers</div><strong class="workspace-value">{{.Service.RunningCount}} / {{.Service.ContainerCount}}</strong><span class="stat-hint">Health: {{.Service.ContainerHealth}}</span></article>
</div></section>
<section class="section panel workspace-detail" aria-labelledby="service-image-title"><h2 id="service-image-title">Image and deployment evidence</h2>
<dl class="digest-list">
<div><dt>Docker-observed container image</dt><dd class="mono">{{if .Service.MixedDiscoveredImages}}Mixed image references across replicas{{else if .Service.DiscoveredImage}}{{.Service.DiscoveredImage}}{{else}}No recent Docker inventory{{end}}</dd></div>
<div><dt>Last Docker inventory scan</dt><dd>{{when .Service.InventoryObservedAt}}</dd></div>
<div><dt>Registry watch image</dt><dd class="mono">{{if .Service.HasPolicy}}{{.Service.Image}}{{else}}Not enrolled{{end}}</dd></div>
<div><dt>Platform</dt><dd>{{if .Service.Platform}}{{.Service.Platform}}{{else}}Unknown{{end}}</dd></div>
<div><dt>Deployed assessment</dt><dd><span class="pill {{tone .Service.Assessment}}">{{status .Service.Assessment}}</span>{{if .Reason}}<small class="subtext">{{.Reason}}</small>{{end}}</dd></div>
<div><dt>Deployed digest</dt><dd class="mono">{{if .DeployedDigest}}{{.DeployedDigest}}{{else}}Not verified{{end}}</dd></div>
<div><dt>Remote digest</dt><dd class="mono">{{if .RemoteDigest}}{{.RemoteDigest}}{{else}}Not observed{{end}}</dd></div>
<div><dt>Inventory observed</dt><dd>{{when .InventoryObservedAt}}</dd></div>
<div><dt>Registry checked</dt><dd>{{when .RegistryCheckedAt}}</dd></div>
</dl>
{{if .Service.ImageID}}<p><a class="text-action" href="{{imageURL .Service.ImageID}}">View image details</a></p>{{end}}
<p class="helper">Docker-observed image is the reference recorded when a container was created. Pulling a newer tag or restarting the same container does not change it. The monitored registry reference can differ, especially for legacy manual policies. The application build is shown in the sidebar.</p>
<p class="helper">A different remote digest does not prove a newer release.</p>
</section>
<section class="section panel workspace-detail" id="monitoring-controls" aria-labelledby="service-monitoring-title"><h2 id="service-monitoring-title">Monitoring configuration</h2>
<dl class="digest-list">
<div><dt>Enrollment source</dt><dd>{{.Service.Enrollment}}</dd></div>
<div><dt>Discovery</dt><dd>{{.Service.DiscoveryState}}{{if .Service.ValidationError}} · {{.Service.ValidationError}}{{end}}</dd></div>
<div><dt>Effective interval</dt><dd>{{.Service.IntervalSeconds}} seconds ({{.Service.IntervalSource}})</dd></div>
<div><dt>Docker label interval</dt><dd>{{with .Service.LabelInterval}}{{.}} seconds{{else}}Not set{{end}}</dd></div>
<div><dt>GUI override</dt><dd>{{with .Service.GUIOverride}}{{.}} seconds{{else}}Not set{{end}}</dd></div>
<div><dt>Last monitoring attempt</dt><dd>{{when .Service.LastCheckedAt}}</dd></div>
<div><dt>Next scheduled check</dt><dd>{{if .Service.HasPolicy}}{{when .Service.NextCheckAt}}{{else}}Not scheduled{{end}}</dd></div>
{{if .Service.LastError}}<div><dt>Last check error</dt><dd class="error-cell">{{.Service.LastError}}</dd></div>{{end}}
</dl>
{{if eq .Service.Monitoring "disabled_by_label"}}<p class="helper status-explanation">Monitoring is explicitly disabled by the Docker Compose label. A GUI override cannot enable it. Change the Compose label to manage enrollment.</p>{{end}}
{{if eq .Service.Monitoring "invalid_discovery"}}<p class="helper status-explanation">Discovery metadata is invalid or conflicting. Correct it before attempting monitoring changes.</p>{{end}}
<div class="service-actions">
{{if .Service.CanGUIEnable}}
<form method="post" action="/settings/monitoring/service"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="project" value="{{.Service.Project}}"><input type="hidden" name="service" value="{{.Service.Service}}"><input type="hidden" name="return_to" value="{{.ReturnTo}}"><button type="submit" name="action" value="enable">Enable monitoring</button></form>
{{end}}
{{if .Service.CanGUIDisable}}
<form method="post" action="/settings/monitoring/service"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="project" value="{{.Service.Project}}"><input type="hidden" name="service" value="{{.Service.Service}}"><input type="hidden" name="return_to" value="{{.ReturnTo}}"><button type="submit" name="action" value="disable">Disable GUI enrollment</button></form>
{{end}}
{{if .Service.CanPause}}
<form method="post" action="/settings/monitoring/service"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="project" value="{{.Service.Project}}"><input type="hidden" name="service" value="{{.Service.Service}}"><input type="hidden" name="return_to" value="{{.ReturnTo}}"><button type="submit" name="action" value="pause">Pause monitoring</button></form>
{{end}}
{{if .Service.CanResume}}
<form method="post" action="/settings/monitoring/service"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="project" value="{{.Service.Project}}"><input type="hidden" name="service" value="{{.Service.Service}}"><input type="hidden" name="return_to" value="{{.ReturnTo}}"><button type="submit" name="action" value="resume">Resume monitoring</button></form>
{{end}}
</div>
{{if .CanOverride}}
<form class="service-interval-form" method="post" action="/settings/monitoring/service">
<input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="project" value="{{.Service.Project}}"><input type="hidden" name="service" value="{{.Service.Service}}"><input type="hidden" name="return_to" value="{{.ReturnTo}}">
<label for="service-interval">Override monitoring interval (seconds, 60–86400)</label>
<div class="service-interval-controls"><input id="service-interval" type="number" name="interval_seconds" min="60" max="86400" value="{{.Service.IntervalSeconds}}" required>
<button type="submit" name="action" value="override">Save override</button>
{{if .CanReset}}<button type="submit" name="action" value="reset" formnovalidate>Reset to label/default</button>{{end}}
</div></form>
{{end}}
{{if not .CanOverride}}<p class="helper">No monitoring configuration actions are currently available for this service.</p>{{end}}
<p class="helper">Precedence: GUI override → Docker label → global default. Monitoring changes never pull images or restart containers.</p>
<p><a class="text-action" href="{{.SettingsURL}}">Manage monitoring settings in Settings</a></p>
</section>
<footer>Monitoring enrollment does not authorize Docker Compose modifications or deployments.</footer>{{template "shell-end" .}}</body></html>`)
