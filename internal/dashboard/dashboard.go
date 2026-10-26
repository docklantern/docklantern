// Package dashboard renders the image-first monitoring overview using read-only
// observations already persisted by Docker discovery and registry workers.
package dashboard

import (
 "context"
 "embed"
 "html/template"
 "net/http"
 "strings"
 "time"

 "github.com/docklantern/docklantern/internal/layout"
 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/store"
 "github.com/docklantern/docklantern/internal/updates"
)

//go:embed assets/dashboard.html assets/dashboard.css
var assets embed.FS

var page = template.Must(layout.WithShell(template.Must(template.New("dashboard.html").Funcs(template.FuncMap{
 "badge":func(s string)string{
  switch strings.ToLower(s){
  case "current","unchanged","ok":return "ok"
  case "changed","different","mixed","check_failed","error","registry_failure","registry_outage_prolonged","remote_digest_changed":return "warn"
  default:return "muted"
  }
 },
 "when":func(s string)string{
  if s==""{return "Not recorded"}
  t,err:=time.Parse(time.RFC3339Nano,s)
  if err!=nil{return "Unknown"}
  return t.UTC().Format("2006-01-02 15:04 UTC")
 },
 "imageURL":func(s string)string{return "/images/"+s},
 "eventKind":presentation.EventKind,
 "eventScope":presentation.EventScope,
}).ParseFS(assets,"assets/dashboard.html"))))

// Reader intentionally retains the existing API-facing interface. Additional
// methods are optional for test doubles and older API readers, while Store
// supplies full persisted change history, activity and operations data.
type Reader interface{
 Projects(context.Context)([]store.Registration,error)
 Containers(context.Context)([]store.Container,error)
 RegistryChecks(context.Context)([]store.RegistryCheck,error)
 MonitorPolicies(context.Context)([]store.MonitorPolicy,error)
}

type activityRow struct {
 Event store.Event
 ImageID string
}

type view struct {
 ActivePage string
 MainClass string
 Metrics presentation.ImageOverviewMetrics
 Review []presentation.ImageReview
 TotalReview int
 Activity []activityRow
 Workers []store.WorkerStatus
 EventHistoryAvailable bool
}

func collect(ctx context.Context,s Reader)(view,error){
 registrations,err:=s.Projects(ctx)
 if err!=nil{return view{},err}
 containers,err:=s.Containers(ctx)
 if err!=nil{return view{},err}
 checks,err:=s.RegistryChecks(ctx)
 if err!=nil{return view{},err}
 policies,err:=s.MonitorPolicies(ctx)
 if err!=nil{return view{},err}
 changes:=[]store.Event{}
 changeHistoryAvailable:=false
 if reader,ok:=s.(interface{LatestImageChangeEvents(context.Context)([]store.Event,error)});ok{
  changes,err=reader.LatestImageChangeEvents(ctx)
  if err!=nil{return view{},err}
  changeHistoryAvailable=true
 }
 now:=time.Now().UTC()
 assessments:=updates.AssessMonitoredFresh(registrations,containers,policies,checks,now)
 images:=presentation.BuildImageSummaries(policies,checks,assessments,changes,now)
 metrics,review,total:=presentation.BuildImageOverview(images,now,8)
 v:=view{ActivePage:"overview",Metrics:metrics,Review:review,TotalReview:total,EventHistoryAvailable:changeHistoryAvailable}

 // Event feed is intentionally bounded and contains no repeated unchanged
 // polls. Image-scoped events do not acquire an arbitrary project/service.
 byImage:=make(map[presentation.ImageIdentity]string,len(images))
 for _,image:=range images{
  byImage[presentation.ImageIdentity{Reference:image.Reference,Platform:image.Platform}]=image.ID
 }
 if reader,ok:=s.(interface{Events(context.Context,int)([]store.Event,error)});ok{
  events,e:=reader.Events(ctx,20)
  if e!=nil{return view{},e}
  for _,event:=range events{
   switch event.Kind{
   case "remote_digest_changed","registry_failure","registry_recovered","registry_outage_prolonged":
   default:continue
   }
   v.Activity=append(v.Activity,activityRow{
    Event:event,ImageID:byImage[presentation.ImageIdentity{Reference:event.Image,Platform:event.Platform}],
   })
   if len(v.Activity)==6{break}
  }
 }
 if reader,ok:=s.(interface{WorkerStatuses(context.Context)([]store.WorkerStatus,error)});ok{
  workers,e:=reader.WorkerStatuses(ctx)
  if e!=nil{return view{},e}
  if len(workers)>3{workers=workers[:3]}
  v.Workers=workers
 }
 return v,nil
}

func Handler(s Reader)http.Handler{
 mux:=http.NewServeMux()
 css,err:=assets.ReadFile("assets/dashboard.css")
 if err!=nil{panic(err)}
 mux.HandleFunc("GET /assets/dashboard.css",func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Content-Type","text/css; charset=utf-8")
  w.Header().Set("Cache-Control","public, max-age=3600")
  w.Header().Set("X-Content-Type-Options","nosniff")
  _,_=w.Write(css)
 })
 mux.HandleFunc("GET /",func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/"{http.NotFound(w,r);return}
  v,err:=collect(r.Context(),s)
  if err!=nil{http.Error(w,"Dashboard unavailable",http.StatusInternalServerError);return}
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("X-Content-Type-Options","nosniff")
  w.Header().Set("Referrer-Policy","no-referrer")
  w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
  _=page.ExecuteTemplate(w,"dashboard.html",v)
 })
 return mux
}
