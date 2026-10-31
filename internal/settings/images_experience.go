package settings

import (
 "errors"
 "net/http"
 "sort"
 "strconv"
 "strings"
 "time"

 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/store"
 "github.com/docklantern/docklantern/internal/updates"
)

type imageFilters struct {
 Query string
 Registry string
 Deployment string
 Sort string
}

type imageListData struct {
 workspaceData
 Filter imageFilters
 Total int
 Visible int
 Changed24h int
 Differences int
 Errors int
}

type imageServiceEvidence struct {
 Service presentation.ServiceSummary
 Reason string
 DeployedDigest string
 RemoteDigest string
 RegistryCheckedAt string
 InventoryObservedAt string
}

type imageDetailsData struct {
 workspaceData
 Evidence []imageServiceEvidence
 History []store.Event
 NextBefore int64
 CSRF string
 CheckNotice string
 CanCheckNow bool
 EligibleWatches int
}

func parseImageFilters(r *http.Request)(imageFilters,error) {
 q:=r.URL.Query()
 f:=imageFilters{
  Query:strings.TrimSpace(q.Get("q")),
  Registry:q.Get("registry"),
  Deployment:q.Get("deployment"),
  Sort:q.Get("sort"),
 }
 if len(f.Query)>128 || strings.ContainsAny(f.Query,"\x00\r\n"){
  return imageFilters{},errors.New("invalid search")
 }
 if f.Sort==""{f.Sort="attention"}
 switch f.Sort{case "attention","name","last_check","services":default:return imageFilters{},errors.New("invalid sort")}
 switch f.Registry{
 case "","changed","recorded_change_24h","unchanged","check_failed","stale","not_checked","first_seen":
 default:return imageFilters{},errors.New("invalid registry status")
 }
 switch f.Deployment{
 case "","current","different","mixed","unknown","not_running":
 default:return imageFilters{},errors.New("invalid deployment status")
 }
 return f,nil
}

func hasRecordedChangeInWindow(image presentation.ImageSummary,now time.Time,window time.Duration)bool{
 return presentation.RecentRecordedChange(image,now,window)
}

func attentionRank(image presentation.ImageSummary)int{
 switch{
 case image.RegistryStatus=="check_failed":return 0
 case image.DeploymentCounts.Different>0:return 1
 case image.RegistryStatus=="changed":return 2
 case image.RegistryStatus=="stale":return 3
 case image.DeploymentStatus=="unknown"||image.DeploymentStatus=="mixed":return 4
 default:return 5
 }
}

func filterImages(images []presentation.ImageSummary,f imageFilters,now time.Time)[]presentation.ImageSummary{
 filtered:=make([]presentation.ImageSummary,0,len(images))
 term:=strings.ToLower(f.Query)
 for _,item:=range images {
  if f.Registry!="" {
   if f.Registry=="recorded_change_24h"{
    if !hasRecordedChangeInWindow(item,now,24*time.Hour){continue}
   }else if item.RegistryStatus!=f.Registry{continue}
  }
  if f.Deployment!="" && item.DeploymentStatus!=f.Deployment{continue}
  if term!="" {
   haystack:=strings.ToLower(item.Reference+" "+item.Platform)
   for _,s:=range item.Services{haystack+=" "+strings.ToLower(s.Project+" "+s.Service)}
   if !strings.Contains(haystack,term){continue}
  }
  filtered=append(filtered,item)
 }
 sort.SliceStable(filtered,func(i,j int)bool{
  a,b:=filtered[i],filtered[j]
  switch f.Sort {
  case "attention":
   if attentionRank(a)!=attentionRank(b){return attentionRank(a)<attentionRank(b)}
  case "services":
   if len(a.Services)!=len(b.Services){return len(a.Services)>len(b.Services)}
  case "last_check":
   aDate,bDate:="",""
   if a.LatestObservation!=nil{aDate=a.LatestObservation.CheckedAt}
   if b.LatestObservation!=nil{bDate=b.LatestObservation.CheckedAt}
   at,_:=time.Parse(time.RFC3339Nano,aDate)
   bt,_:=time.Parse(time.RFC3339Nano,bDate)
   if !at.Equal(bt){return at.After(bt)}
  }
  if a.Reference!=b.Reference{return a.Reference<b.Reference}
  return a.Platform<b.Platform
 })
 return filtered
}

func(h Handler) renderImagesWorkspace(w http.ResponseWriter,r *http.Request){
 filters,err:=parseImageFilters(r)
 if err!=nil{http.Error(w,"Invalid image filters",http.StatusBadRequest);return}
 data,err:=h.workspaceSnapshot(r.Context())
 if err!=nil{http.Error(w,"Images unavailable",http.StatusInternalServerError);return}
 now:=time.Now().UTC()
 d:=imageListData{workspaceData:data,Filter:filters,Total:len(data.Images)}
 metrics,_,_:=presentation.BuildImageOverview(data.Images,now,0)
 d.Changed24h=metrics.RemoteChanges24h
 d.Differences=metrics.DeploymentDifferences
 d.Errors=metrics.MonitoringErrors
 d.Images=filterImages(data.Images,filters,now)
 d.Visible=len(d.Images)
 d.ActivePage="images"
 workspaceHeaders(w)
 _=imagesExperienceTemplate.Execute(w,d)
}

func(h Handler) renderImageWorkspaceDetails(w http.ResponseWriter,r *http.Request){
 identity,err:=presentation.ParseImageID(r.PathValue("id"))
 if err!=nil{http.NotFound(w,r);return}
 notice,valid:=checkNotice(r.URL.Query().Get("check"))
 if !valid{http.Error(w,"Invalid check status",http.StatusBadRequest);return}
 before:=int64(0)
 if raw:=r.URL.Query().Get("before");raw!=""{
  before,err=strconv.ParseInt(raw,10,64)
  if err!=nil||before<=0{http.Error(w,"Invalid event cursor",http.StatusBadRequest);return}
 }
 data,err:=h.workspaceSnapshot(r.Context())
 if err!=nil{http.Error(w,"Image unavailable",http.StatusInternalServerError);return}
 var match *presentation.ImageSummary
 for i:=range data.Images{
  candidate:=&data.Images[i]
  if candidate.Reference==identity.Reference && candidate.Platform==identity.Platform{
   match=candidate
   break
  }
 }
 if match==nil{http.NotFound(w,r);return}
 data.Image=match
 data.ActivePage="images"
 watch:=make(map[presentation.ServiceIdentity]bool,len(match.Services))
 for _,id:=range match.Services{watch[id]=true}
 for _,s:=range data.Services{
  if watch[s.ServiceIdentity]{data.Associated=append(data.Associated,s)}
 }
 assess:=make(map[presentation.ServiceIdentity]updates.Assessment,len(data.Assessments))
 for _,a:=range data.Assessments{
  if a.Image==identity.Reference && a.Platform==identity.Platform{
   assess[presentation.ServiceIdentity{Project:a.Project,Service:a.Service}]=a
  }
 }
 details:=imageDetailsData{workspaceData:data,CSRF:h.Guard.Token(),CheckNotice:notice}
 for _,s:=range data.Associated{
  if s.CanCheckNow{details.EligibleWatches++}
  row:=imageServiceEvidence{Service:s}
  if a,ok:=assess[s.ServiceIdentity];ok{
   row.Reason=a.Reason
   row.DeployedDigest=a.DeployedDigest
   row.RemoteDigest=a.RemoteDigest
   row.RegistryCheckedAt=a.RegistryCheckedAt
   row.InventoryObservedAt=a.InventoryObservedAt
  }
  details.Evidence=append(details.Evidence,row)
 }
 // Read 26 entries to determine whether there is a next page of 25.
 events,err:=h.Backend.ImageEvents(r.Context(),identity.Reference,identity.Platform,before,26)
 if err!=nil{http.Error(w,"Image history unavailable",http.StatusInternalServerError);return}
 if len(events)>25{
  events=events[:25]
  details.NextBefore=events[len(events)-1].ID
 }
 details.History=events
 details.CanCheckNow=details.EligibleWatches>0
 workspaceHeaders(w)
 _=imageExperienceDetailTemplate.Execute(w,details)
}

var imagesExperienceTemplate=workspaceTemplate("images-v2",`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>Images · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head><body>
{{template "shell-start" .}}
<header class="topbar"><div><p class="eyebrow">DOCKER COMPOSE / IMAGES</p><h1>Images</h1><p class="muted">Monitored image references, registry observations and deployed-image comparisons.</p></div><span class="readonly">MONITORING ONLY</span></header>
<section class="section" aria-label="Image monitoring summary"><div class="stats image-stats">
<article class="stat-card"><div class="stat-label">Watched images</div><strong>{{.Total}}</strong><span class="stat-hint">Unique reference and platform pairs</span></article>
<article class="stat-card"><div class="stat-label">Recorded changes (24h)</div><strong>{{.Changed24h}}</strong><span class="stat-hint">Distinct images with retained change events</span></article>
<article class="stat-card"><div class="stat-label">Verified differences</div><strong>{{.Differences}}</strong><span class="stat-hint">Images with at least one differing deployment</span></article>
<article class="stat-card"><div class="stat-label">Monitoring errors</div><strong>{{.Errors}}</strong><span class="stat-hint">Images with a failed service check</span></article>
</div></section>
<section class="section" aria-labelledby="watched-images-title">
<div class="section-head"><div><h2 id="watched-images-title">Watched images</h2><p class="helper">Registry status describes observations; deployed status describes verifiable running image evidence. Neither proves a newer software release.</p></div><span class="helper">{{.Visible}} of {{.Total}} images</span></div>
<form class="image-filters panel" action="/images" method="get" role="search">
<label>Search image, platform or service<input type="search" name="q" maxlength="128" value="{{.Filter.Query}}" placeholder="nginx, ghcr.io, project..."></label>
<label>Registry status<select name="registry"><option value="">All registry states</option>
<option value="recorded_change_24h" {{if eq .Filter.Registry "recorded_change_24h"}}selected{{end}}>Recorded change in 24h</option>
<option value="changed" {{if eq .Filter.Registry "changed"}}selected{{end}}>Latest check changed</option>
<option value="unchanged" {{if eq .Filter.Registry "unchanged"}}selected{{end}}>Latest check unchanged</option>
<option value="check_failed" {{if eq .Filter.Registry "check_failed"}}selected{{end}}>Check failed</option>
<option value="stale" {{if eq .Filter.Registry "stale"}}selected{{end}}>Stale</option>
<option value="not_checked" {{if eq .Filter.Registry "not_checked"}}selected{{end}}>Not checked</option>
<option value="first_seen" {{if eq .Filter.Registry "first_seen"}}selected{{end}}>First observed</option></select></label>
<label>Deployed status<select name="deployment"><option value="">All deployed states</option>
<option value="current" {{if eq .Filter.Deployment "current"}}selected{{end}}>Current</option>
<option value="different" {{if eq .Filter.Deployment "different"}}selected{{end}}>Different</option>
<option value="mixed" {{if eq .Filter.Deployment "mixed"}}selected{{end}}>Mixed</option>
<option value="unknown" {{if eq .Filter.Deployment "unknown"}}selected{{end}}>Unknown</option>
<option value="not_running" {{if eq .Filter.Deployment "not_running"}}selected{{end}}>Not running</option></select></label>
<label>Sort by<select name="sort"><option value="attention" {{if eq .Filter.Sort "attention"}}selected{{end}}>Needs attention</option>
<option value="name" {{if eq .Filter.Sort "name"}}selected{{end}}>Image name</option>
<option value="last_check" {{if eq .Filter.Sort "last_check"}}selected{{end}}>Last successful check</option>
<option value="services" {{if eq .Filter.Sort "services"}}selected{{end}}>Most services</option></select></label>
<div class="image-filter-actions"><button type="submit">Apply filters</button><a class="text-action" href="/images">Clear</a></div>
</form>
<div class="panel table-scroll"><table><thead><tr><th>Image / platform</th><th>Registry status</th><th>Deployment status</th><th>Services</th><th>Last successful check</th><th>Details</th></tr></thead><tbody>
{{range .Images}}
<tr><td><strong class="mono image-ref">{{.Reference}}</strong><small class="subtext">{{.Platform}}</small></td>
<td><span class="pill {{tone .RegistryStatus}}">{{status .RegistryStatus}}</span>{{if .LastChangeAt}}<small class="subtext">Last recorded change: {{when .LastChangeAt}}</small>{{end}}</td>
<td><span class="pill {{tone .DeploymentStatus}}">{{status .DeploymentStatus}}</span>{{if .DeploymentCounts.Different}}<small class="subtext">{{.DeploymentCounts.Different}} verified differing deployment(s)</small>{{end}}</td>
<td>{{len .Services}}</td>
<td>{{with .LatestObservation}}{{when .CheckedAt}}{{else}}Not yet{{end}}</td>
<td><a class="text-action" href="{{imageURL .ID}}">View image</a></td></tr>
{{else}}<tr><td colspan="6" class="table-empty">{{if .Total}}No images match these filters. <a href="/images">Clear filters</a>.{{else}}No watched images yet. Enable monitoring on a discovered Compose service.{{end}}</td></tr>{{end}}
</tbody></table></div></section>
<footer>Remote digest changes do not prove a newer release · No automatic image pulls or deployments.</footer>{{template "shell-end" .}}</body></html>`)

var imageExperienceDetailTemplate=workspaceTemplate("image-detail-v2",`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>Image Details · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head><body>
{{template "shell-start" .}}
<nav class="breadcrumbs" aria-label="Breadcrumb"><a href="/images">Images</a><span aria-hidden="true">/</span><span aria-current="page">Image Details</span></nav>
<header class="topbar"><div><p class="eyebrow">IMAGE DETAILS</p><h1 class="workspace-title mono">{{.Image.Reference}}</h1><p class="muted">Platform: {{.Image.Platform}}</p></div><span class="readonly">MONITORING ONLY</span></header>
{{if .CheckNotice}}<p class="notice" role="status">{{.CheckNotice}}</p>{{end}}
<section class="section" aria-label="Image assessment summary"><div class="workspace-stats">
<article class="stat-card"><div class="stat-label">Registry status</div><strong class="workspace-value"><span class="pill {{tone .Image.RegistryStatus}}">{{status .Image.RegistryStatus}}</span></strong><span class="stat-hint">{{with .Image.LatestObservation}}Last successful check: {{when .CheckedAt}}{{else}}No successful registry check recorded{{end}}</span></article>
<article class="stat-card"><div class="stat-label">Deployed status</div><strong class="workspace-value"><span class="pill {{tone .Image.DeploymentStatus}}">{{status .Image.DeploymentStatus}}</span></strong><span class="stat-hint">{{.Image.DeploymentCounts.Current}} current · {{.Image.DeploymentCounts.Different}} different · {{.Image.DeploymentCounts.Unknown}} unknown · {{.Image.DeploymentCounts.NotRunning}} not running</span></article>
<article class="stat-card"><div class="stat-label">Associated services</div><strong class="workspace-value">{{len .Image.Services}}</strong><span class="stat-hint">{{if .Image.LastChangeAt}}Last recorded change: {{when .Image.LastChangeAt}}{{else}}No retained digest-change event{{end}}</span></article>
</div></section>
<section class="section panel workspace-detail" aria-labelledby="image-check-title">
<h2 id="image-check-title">Registry check</h2>
<p class="helper">Check Now requests an observation for eligible services watching this exact image and platform, not all containers on the host. Registry backoff and request cooldown still apply.</p>
{{if .CanCheckNow}}
<p class="helper">{{.EligibleWatches}} eligible watcher(s) currently associated with this image.</p>
<form method="post" action="/settings/monitoring/check-now/image">
<input type="hidden" name="csrf_token" value="{{.CSRF}}">
<input type="hidden" name="image_id" value="{{.Image.ID}}">
<button type="submit">Check Now</button>
</form>
{{else}}<p class="helper">No eligible watches are currently active for this image. Paused, disabled and invalid services cannot be forced to check.</p>{{end}}
</section>
<section class="section panel workspace-detail" aria-labelledby="registry-evidence-title"><h2 id="registry-evidence-title">Registry evidence</h2>
<p class="helper">Remote OCI manifest identity, not a verified newer application version.</p>
{{with .Image.LatestObservation}}
<dl class="digest-list"><div><dt>Registry reference</dt><dd class="mono">{{.Image}}</dd></div>
<div><dt>Platform</dt><dd>{{.Platform}}</dd></div>
<div><dt>Index digest</dt><dd class="mono">{{.IndexDigest}}</dd></div>
<div><dt>Platform digest</dt><dd class="mono">{{.PlatformDigest}}</dd></div>
<div><dt>Last successful check</dt><dd>{{when .CheckedAt}}</dd></div>
<div><dt>Last successful result</dt><dd>{{status .Status}}</dd></div></dl>
{{else}}<p class="muted">No successful remote registry observation has been saved yet.</p>{{end}}
{{if .Image.FailedServices}}<p class="helper status-explanation">{{.Image.FailedServices}} service check(s) currently report an error. Last successful evidence remains displayed above.</p>{{end}}
</section>
<section class="section" aria-labelledby="image-services-title"><div class="section-head"><div><h2 id="image-services-title">Associated services</h2><p class="helper">Service-specific monitoring and immutable deployed-image evidence.</p></div></div>
<div class="panel table-scroll"><table><thead><tr><th>Compose service</th><th>Monitoring</th><th>Deployed assessment</th><th>Deployed digest evidence</th><th>Details</th></tr></thead><tbody>
{{range .Evidence}}<tr><td><strong>{{.Service.Project}} / {{.Service.Service}}</strong></td>
<td>{{status .Service.Monitoring}}
<small class="subtext">Last attempt: {{when .Service.LastCheckedAt}}</small>
{{if .Service.NextCheckAt}}<small class="subtext">Next: {{when .Service.NextCheckAt}}</small>{{end}}
{{if .Service.ConsecutiveFailures}}<small class="subtext">Failures: {{.Service.ConsecutiveFailures}} (retry backoff may apply)</small>{{end}}
</td>
<td><span class="pill {{tone .Service.Assessment}}">{{status .Service.Assessment}}</span>{{if .Reason}}<small class="subtext">{{.Reason}}</small>{{end}}</td>
<td>{{if .DeployedDigest}}<span class="mono" title="{{.DeployedDigest}}">{{shortDigest .DeployedDigest}}</span>{{else}}<span class="muted">Not verified</span>{{end}}</td>
<td><a class="text-action" href="{{serviceURL .Service.Project .Service.Service}}">View service</a></td></tr>
{{else}}<tr><td colspan="5" class="table-empty">No associated discovered service details are available.</td></tr>{{end}}</tbody></table></div></section>
<section class="section" aria-labelledby="image-history-title"><div class="section-head"><div><h2 id="image-history-title">Registry history</h2><p class="helper">Retained events scoped to this exact image and platform; newest first. Event history may be pruned.</p></div></div>
<div class="panel table-scroll"><table><thead><tr><th>Recorded (UTC)</th><th>Observation</th><th>Digest evidence</th></tr></thead><tbody>
{{range .History}}<tr><td>{{when .OccurredAt}}</td><td>{{eventKind .Kind}}</td>
<td>{{if .PreviousDigest}}<small class="subtext">Previous: <span class="mono" title="{{.PreviousDigest}}">{{shortDigest .PreviousDigest}}</span></small>{{end}}
{{if .Digest}}<small class="subtext">Observed: <span class="mono" title="{{.Digest}}">{{shortDigest .Digest}}</span></small>{{end}}
{{if and (not .Digest) (not .PreviousDigest)}}<span class="muted">No digest evidence recorded</span>{{end}}</td></tr>
{{else}}<tr><td colspan="3" class="table-empty">No retained events for this image and platform.</td></tr>{{end}}
</tbody></table></div>
{{if .NextBefore}}<nav aria-label="Registry event pages"><a class="text-action" href="/images/{{.Image.ID}}?before={{.NextBefore}}">Older registry events</a></nav>{{end}}
</section>
<footer>Registry observation is not software release classification · No Docker updates or deployments.</footer>{{template "shell-end" .}}</body></html>`)
