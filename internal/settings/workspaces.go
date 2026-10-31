package settings

import (
 "context"
 "html/template"
 "net/http"
 "net/url"
 "time"

 "github.com/docklantern/docklantern/internal/layout"
 "github.com/docklantern/docklantern/internal/presentation"
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
 Assessments []updates.Assessment
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
 "tone":func(s string)string{
  switch s {
  case "current","unchanged":return "ok"
  case "different","mixed","changed","check_failed":return "warn"
  default:return "muted"
  }
 },
 "eventKind":presentation.EventKind,
 "shortDigest":presentation.ShortDigest,
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

// All workspace views use the existing store and OCI assessment code; these
// GET handlers never contact the registry, write SQLite, or execute Docker.
func(h Handler) workspaceSnapshot(ctx context.Context)(workspaceData,error){
 projects,err:=h.Backend.Projects(ctx);if err!=nil{return workspaceData{},err}
 containers,err:=h.Backend.Containers(ctx);if err!=nil{return workspaceData{},err}
 policies,err:=h.Backend.MonitorPolicies(ctx);if err!=nil{return workspaceData{},err}
 checks,err:=h.Backend.RegistryChecks(ctx);if err!=nil{return workspaceData{},err}
 views,err:=h.Backend.MonitorServiceViews(ctx);if err!=nil{return workspaceData{},err}
 events,err:=h.Backend.LatestImageChangeEvents(ctx)
 if err!=nil{return workspaceData{},err}
 now:=time.Now().UTC()
 assessments:=updates.AssessMonitoredFresh(projects,containers,policies,checks,now)
 result:=workspaceData{
  Images:presentation.BuildImageSummaries(policies,checks,assessments,events,now),
  Services:presentation.BuildServiceSummaries(views,containers,policies,assessments),
  Assessments:assessments,
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
 w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'self'; img-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
}
func renderWorkspace(w http.ResponseWriter,t *template.Template,data workspaceData){
 workspaceHeaders(w)
 _=t.Execute(w,data)
}
func(h Handler) imagesPage(w http.ResponseWriter,r *http.Request){h.renderImagesWorkspace(w,r)}
func(h Handler) imageDetailsPage(w http.ResponseWriter,r *http.Request){h.renderImageWorkspaceDetails(w,r)}
func(h Handler) servicesPage(w http.ResponseWriter,r *http.Request){h.renderServicesWorkspace(w,r)}
func(h Handler) serviceDetailsPage(w http.ResponseWriter,r *http.Request){h.renderServiceWorkspaceDetails(w,r)}

// checkNotice translates backend scheduling outcomes into safe, user-facing
// status messages. Only known values are accepted in GET query parameters;
// the redirect is NOT evidence that a registry check has completed.
func checkNotice(code string)(string,bool){
 switch code {
 case "":return "",true
 case "scheduled":return "Registry check scheduled. The background monitor will process it; no result is available yet.",true
 case "already_due":return "A registry check is already due. No duplicate request was scheduled.",true
 case "deferred_backoff":return "Check deferred: an existing registry failure or rate-limit backoff must expire first.",true
 case "deferred_recent":return "Check deferred: this image was requested or checked recently.",true
 default:return "",false
 }
}
