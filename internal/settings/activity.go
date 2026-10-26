package settings

import (
 "html/template"
 "net/http"
 "strconv"
 "time"

 "github.com/docklantern/docklantern/internal/layout"
 "github.com/docklantern/docklantern/internal/presentation"
 "github.com/docklantern/docklantern/internal/store"
)

type activityEntry struct{
 Event store.Event
 ImageID string
}

type activityGroup struct{
 Day string
 Entries []activityEntry
}

type activityPageData struct{
 ActivePage string
 MainClass string
 Groups []activityGroup
 Kind string
 DeliveryState string
 Search string
 NextBefore int64
 HasEvents bool
}

var activityHTML=template.Must(layout.WithShell(template.Must(template.New("activity").Funcs(template.FuncMap{
 "eventKind":presentation.EventKind,
 "eventScope":presentation.EventScope,
 "eventDelivery":presentation.EventDelivery,
 "shortDigest":presentation.ShortDigest,
 "imageURL":func(id string)string{return "/images/"+id},
 "eventTone":func(kind string)string{
  switch kind{case "registry_recovered":return "ok";case "remote_digest_changed","registry_failure","registry_outage_prolonged":return "warn";default:return "muted"}
 },
 "when":func(stamp string)string{
  when,err:=time.Parse(time.RFC3339Nano,stamp)
  if err!=nil{return "Unknown time"}
  return when.UTC().Format("15:04 UTC")
 },
}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>Activity · DockLantern</title><link rel="stylesheet" href="/assets/dashboard.css"></head><body>
{{template "shell-start" .}}
<header class="topbar"><div><p class="eyebrow">DOCKER COMPOSE / ACTIVITY</p><h1>Activity</h1><p class="muted">Registry digest changes, monitoring failures, recoveries and event history.</p></div><span class="readonly">MONITORING ONLY</span></header>
<section class="section" aria-label="Event filters">
<nav class="activity-quick-filters" aria-label="Quick activity filters"><a href="/events" {{if eq .Kind ""}}aria-current="page"{{end}}>All events</a><a href="/events?kind=remote_digest_changed" {{if eq .Kind "remote_digest_changed"}}aria-current="page"{{end}}>Digest changes</a><a href="/events?kind=registry_failure" {{if eq .Kind "registry_failure"}}aria-current="page"{{end}}>Check failures</a><a href="/events?kind=registry_recovered" {{if eq .Kind "registry_recovered"}}aria-current="page"{{end}}>Recoveries</a></nav>
<form class="activity-filters panel" method="get" action="/events" role="search">
<label>Event type<select name="kind"><option value="">All types</option><option value="remote_digest_changed" {{if eq .Kind "remote_digest_changed"}}selected{{end}}>Digest changed</option><option value="registry_failure" {{if eq .Kind "registry_failure"}}selected{{end}}>Registry failure</option><option value="registry_recovered" {{if eq .Kind "registry_recovered"}}selected{{end}}>Recovered</option><option value="registry_outage_prolonged" {{if eq .Kind "registry_outage_prolonged"}}selected{{end}}>Prolonged outage</option></select></label>
<label>Delivery status<select name="delivery"><option value="">All</option><option value="pending" {{if eq .DeliveryState "pending"}}selected{{end}}>Pending</option><option value="delivered" {{if eq .DeliveryState "delivered"}}selected{{end}}>Delivered</option></select></label>
<label>Search project, service or image<input name="search" maxlength="128" value="{{.Search}}" placeholder="registry image, Compose project..."></label>
<div class="activity-filter-actions"><button type="submit">Apply filters</button><a class="text-action" href="/events">Clear</a></div>
</form>
<p class="helper">Registry digest-change events are image/platform scoped and can affect multiple services. Digest changes do not confirm newer software versions.</p>
</section>
<section class="section" aria-labelledby="activity-history-heading">
<div class="section-head"><div><h2 id="activity-history-heading">Monitoring history</h2><p class="helper">Newest first · Retained events · Routine unchanged checks do not create digest-change events.</p></div></div>
{{range .Groups}}
<div class="activity-day"><h3>{{.Day}}</h3><ol class="activity-feed">
{{range .Entries}}<li class="activity-entry">
<div class="activity-entry-icon"><span class="pill {{eventTone .Event.Kind}}">{{eventKind .Event.Kind}}</span></div>
<div class="activity-entry-main"><strong>{{if .ImageID}}<a href="{{imageURL .ImageID}}">{{.Event.Image}}</a>{{else}}{{if .Event.Image}}{{.Event.Image}}{{else}}{{eventScope .Event}}{{end}}{{end}}</strong>
<p class="subtext">{{if .Event.Platform}}{{.Event.Platform}} · {{end}}{{eventScope .Event}}</p>
{{if or .Event.PreviousDigest .Event.Digest}}<div class="activity-evidence">
{{if .Event.PreviousDigest}}<span>Previous: <code title="{{.Event.PreviousDigest}}">{{shortDigest .Event.PreviousDigest}}</code></span>{{end}}
{{if .Event.Digest}}<span>Current: <code title="{{.Event.Digest}}">{{shortDigest .Event.Digest}}</code></span>{{end}}
</div>{{end}}
<details class="activity-delivery"><summary>Webhook delivery details</summary>
<p>{{eventDelivery .Event}}. Webhook delivery is independent of registry checks; pending with zero attempts can mean no notifier is configured.</p>
{{if .Event.NextDeliveryAt}}<p>Next delivery: {{.Event.NextDeliveryAt}}</p>{{end}}
{{if .Event.LastDeliveryError}}<p>Last delivery error: {{.Event.LastDeliveryError}}</p>{{end}}
</details></div><time class="activity-entry-time">{{when .Event.OccurredAt}}</time>
</li>{{end}}</ol></div>{{else}}<div class="panel empty"><h3>No events match these filters</h3><p>Change the filters or <a href="/events">browse all retained activity</a>.</p></div>{{end}}
</section>
{{if .NextBefore}}<nav class="activity-pagination" aria-label="Activity pages"><form method="get" action="/events"><input type="hidden" name="kind" value="{{.Kind}}"><input type="hidden" name="delivery" value="{{.DeliveryState}}"><input type="hidden" name="search" value="{{.Search}}"><input type="hidden" name="before" value="{{.NextBefore}}"><button type="submit">Older events</button></form></nav>{{end}}
<footer>Monitoring observations only · No automatic Docker image pulls or deployments.</footer>
{{template "shell-end" .}}</body></html>`))))

func groupActivityEvents(events []store.Event,watched map[presentation.ImageIdentity]string)[]activityGroup{
 groups:=[]activityGroup{}
 for _,e:=range events{
  parsed,err:=time.Parse(time.RFC3339Nano,e.OccurredAt)
  day:="Unknown date"
  if err==nil{day=parsed.UTC().Format("Monday, January 2, 2006")}
  if len(groups)==0 || groups[len(groups)-1].Day!=day{groups=append(groups,activityGroup{Day:day})}
  i:=len(groups)-1
  groups[i].Entries=append(groups[i].Entries,activityEntry{
   Event:e,ImageID:watched[presentation.ImageIdentity{Reference:e.Image,Platform:e.Platform}],
  })
 }
 return groups
}
func(h Handler) eventsPage(w http.ResponseWriter,r *http.Request){
 q:=r.URL.Query()
 before:=int64(0)
 if q.Has("before"){
  parsed,err:=strconv.ParseInt(q.Get("before"),10,64)
  if err!=nil||parsed<=0{http.Error(w,"Invalid cursor",http.StatusBadRequest);return}
  before=parsed
 }
 // Fetch one extra event to make pagination accurate without an N+1 query.
 f:=store.EventFilter{Kind:q.Get("kind"),DeliveryState:q.Get("delivery"),Search:q.Get("search"),BeforeID:before,Limit:51}
 events,err:=h.Backend.FilterEvents(r.Context(),f)
 if err!=nil{http.Error(w,"Invalid event filter",http.StatusBadRequest);return}
 next:=int64(0)
 if len(events)>50{events=events[:50];next=events[len(events)-1].ID}
 // Only link image-scoped observations to currently watched image identities.
 policies,err:=h.Backend.MonitorPolicies(r.Context())
 if err!=nil{http.Error(w,"Activity unavailable",http.StatusInternalServerError);return}
 watched:=make(map[presentation.ImageIdentity]string,len(policies))
 for _,p:=range policies{
  if p.Image!=""&&p.Platform!=""{
   identity:=presentation.ImageIdentity{Reference:p.Image,Platform:p.Platform}
   watched[identity]=presentation.ImageID(identity)
  }
 }
 data:=activityPageData{
  ActivePage:"events",MainClass:"activity-page",
  Groups:groupActivityEvents(events,watched),
  Kind:f.Kind,DeliveryState:f.DeliveryState,Search:f.Search,
  NextBefore:next,HasEvents:len(events)>0,
 }
 w.Header().Set("Content-Type","text/html; charset=utf-8")
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("Referrer-Policy","no-referrer")
 w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
 _=activityHTML.Execute(w,data)
}
