// Package api exposes monitoring-only inventory endpoints.
package api

import (
 "context"
 "encoding/json"
 "net/http"
 "time"
 "strconv"
 "github.com/docklantern/docklantern/internal/store"
 "github.com/docklantern/docklantern/internal/dashboard"
 "github.com/docklantern/docklantern/internal/updates"
)
type Reader interface{
 Projects(context.Context)([]store.Registration,error)
 Containers(context.Context)([]store.Container,error)
 RegistryChecks(context.Context)([]store.RegistryCheck,error)
 MonitorPolicies(context.Context)([]store.MonitorPolicy,error)
}
func Handler(s Reader)http.Handler{
 mux:=http.NewServeMux()
 mux.Handle("/",dashboard.Handler(s))
 mux.HandleFunc("GET /healthz",func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","application/json");_,_=w.Write([]byte("{\"status\":\"ok\"}"))})
 mux.HandleFunc("GET /api/v1/projects",func(w http.ResponseWriter,r *http.Request){v,e:=s.Projects(r.Context());respond(w,v,e)})
 mux.HandleFunc("GET /api/v1/containers",func(w http.ResponseWriter,r *http.Request){v,e:=s.Containers(r.Context());respond(w,v,e)})
 mux.HandleFunc("GET /api/v1/events",func(w http.ResponseWriter,r *http.Request){
  reader,ok:=s.(interface{Events(context.Context,int)([]store.Event,error)})
  if !ok{respond(w,[]store.Event{},nil);return}
  events,e:=reader.Events(r.Context(),100);respond(w,events,e)
 })
 mux.HandleFunc("GET /api/v1/events/search",func(w http.ResponseWriter,r *http.Request){
  reader,ok:=s.(interface{FilterEvents(context.Context,store.EventFilter)([]store.Event,error)})
  if !ok{respond(w,[]store.Event{},nil);return}
  q:=r.URL.Query()
  before:=int64(0)
  if q.Has("before"){v,e:=strconv.ParseInt(q.Get("before"),10,64);if e!=nil||v<=0{http.Error(w,"invalid event cursor",400);return};before=v}
  limit:=50
  if q.Has("limit"){v,e:=strconv.Atoi(q.Get("limit"));if e!=nil||v<1||v>200{http.Error(w,"invalid event limit",400);return};limit=v}
  data,e:=reader.FilterEvents(r.Context(),store.EventFilter{Kind:q.Get("kind"),DeliveryState:q.Get("delivery"),Search:q.Get("search"),BeforeID:before,Limit:limit})
  if e!=nil{http.Error(w,"invalid event filter",400);return}
  respond(w,data,nil)
 })
 mux.HandleFunc("GET /api/v1/workers",func(w http.ResponseWriter,r *http.Request){
  reader,ok:=s.(interface{WorkerStatuses(context.Context)([]store.WorkerStatus,error)})
  if !ok{respond(w,[]store.WorkerStatus{},nil);return}
  states,err:=reader.WorkerStatuses(r.Context());respond(w,states,err)
 })
 mux.HandleFunc("GET /api/v1/registry-checks",func(w http.ResponseWriter,r *http.Request){v,e:=s.RegistryChecks(r.Context());respond(w,v,e)})
 mux.HandleFunc("GET /api/v1/monitor-policies",func(w http.ResponseWriter,r *http.Request){v,e:=s.MonitorPolicies(r.Context());respond(w,v,e)})
 mux.HandleFunc("GET /api/v1/image-assessments",func(w http.ResponseWriter,r *http.Request){
  ctx:=r.Context();projects,e:=s.Projects(ctx);if e!=nil{respond(w,nil,e);return}
  containers,e:=s.Containers(ctx);if e!=nil{respond(w,nil,e);return}
  policies,e:=s.MonitorPolicies(ctx);if e!=nil{respond(w,nil,e);return}
  checks,e:=s.RegistryChecks(ctx);if e!=nil{respond(w,nil,e);return}
  respond(w,updates.AssessFresh(projects,containers,policies,checks,time.Now().UTC()),nil)
 })
 return mux
}
func respond(w http.ResponseWriter,v any,err error){
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 if err!=nil{http.Error(w,"inventory unavailable",http.StatusInternalServerError);return}
 _=json.NewEncoder(w).Encode(v)
}
