// Package api exposes monitoring-only inventory endpoints.
package api

import (
 "context"
 "encoding/json"
 "net/http"
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
 mux.HandleFunc("GET /api/v1/registry-checks",func(w http.ResponseWriter,r *http.Request){v,e:=s.RegistryChecks(r.Context());respond(w,v,e)})
 mux.HandleFunc("GET /api/v1/monitor-policies",func(w http.ResponseWriter,r *http.Request){v,e:=s.MonitorPolicies(r.Context());respond(w,v,e)})
 mux.HandleFunc("GET /api/v1/image-assessments",func(w http.ResponseWriter,r *http.Request){
  ctx:=r.Context();projects,e:=s.Projects(ctx);if e!=nil{respond(w,nil,e);return}
  containers,e:=s.Containers(ctx);if e!=nil{respond(w,nil,e);return}
  policies,e:=s.MonitorPolicies(ctx);if e!=nil{respond(w,nil,e);return}
  checks,e:=s.RegistryChecks(ctx);if e!=nil{respond(w,nil,e);return}
  respond(w,updates.Assess(projects,containers,policies,checks),nil)
 })
 return mux
}
func respond(w http.ResponseWriter,v any,err error){
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 if err!=nil{http.Error(w,"inventory unavailable",http.StatusInternalServerError);return}
 _=json.NewEncoder(w).Encode(v)
}
