// Package api exposes monitoring-only inventory endpoints.
package api

import (
 "context"
 "encoding/json"
 "net/http"
 "github.com/docklantern/docklantern/internal/store"
)
type Reader interface{
 Projects(context.Context)([]store.Registration,error)
 Containers(context.Context)([]store.Container,error)
 RegistryChecks(context.Context)([]store.RegistryCheck,error)
}
func Handler(s Reader)http.Handler{
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /healthz",func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","application/json");_,_=w.Write([]byte("{\"status\":\"ok\"}"))})
 mux.HandleFunc("GET /api/v1/projects",func(w http.ResponseWriter,r *http.Request){v,e:=s.Projects(r.Context());respond(w,v,e)})
 mux.HandleFunc("GET /api/v1/containers",func(w http.ResponseWriter,r *http.Request){v,e:=s.Containers(r.Context());respond(w,v,e)})
 mux.HandleFunc("GET /api/v1/registry-checks",func(w http.ResponseWriter,r *http.Request){v,e:=s.RegistryChecks(r.Context());respond(w,v,e)})
 return mux
}
func respond(w http.ResponseWriter,v any,err error){
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 if err!=nil{http.Error(w,"inventory unavailable",http.StatusInternalServerError);return}
 _=json.NewEncoder(w).Encode(v)
}
