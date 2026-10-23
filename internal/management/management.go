package management

import (
 "net/http"
 appRuntime "github.com/docklantern/docklantern/internal/runtime"

 "github.com/docklantern/docklantern/internal/api"
 "github.com/docklantern/docklantern/internal/auth"
 "github.com/docklantern/docklantern/internal/settings"
 "github.com/docklantern/docklantern/internal/store"
)
type Backend interface{
 api.Reader
 settings.Backend
}
func Handler(s Backend,g *auth.Guard,roots []string,health ...appRuntime.Checker)http.Handler{
 mux:=http.NewServeMux()
 mux.Handle("/",api.Handler(s))
 mux.HandleFunc("GET /readyz",func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("Content-Type","application/json")
  if len(health)==0{http.Error(w,"not configured",http.StatusServiceUnavailable);return}
  if err:=health[0].Ready(r.Context());err!=nil{
   http.Error(w,`{"status":"not_ready"}`,http.StatusServiceUnavailable);return
  }
  w.WriteHeader(http.StatusOK)
  _,_=w.Write([]byte(`{"status":"ready"}`))
 })
 (settings.Handler{Backend:s,Guard:g,Roots:roots}).Routes(mux)
 return g.Wrap(mux)
}
// Compile-time check: SQL store implements both required interfaces.
var _ Backend=(*store.Store)(nil)
