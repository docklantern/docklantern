package management

import (
 "net/http"

 "github.com/docklantern/docklantern/internal/api"
 "github.com/docklantern/docklantern/internal/auth"
 "github.com/docklantern/docklantern/internal/settings"
 "github.com/docklantern/docklantern/internal/store"
)
type Backend interface{
 api.Reader
 settings.Backend
}
func Handler(s Backend,g *auth.Guard,roots []string)http.Handler{
 mux:=http.NewServeMux()
 mux.Handle("/",api.Handler(s))
 (settings.Handler{Backend:s,Guard:g,Roots:roots}).Routes(mux)
 return g.Wrap(mux)
}
// Compile-time check: SQL store implements both required interfaces.
var _ Backend=(*store.Store)(nil)
