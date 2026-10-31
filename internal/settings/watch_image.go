package settings

import (
 "net/http"
 "net/url"
 "strings"
)

// The store owns all watch-target validation, enrollment and repository
// matching rules. HTTP input is only a bounded service identity and an
// explicit tagged reference; no Docker command or registry URL is executed.
func(h Handler) saveWatchedImage(w http.ResponseWriter,r *http.Request){
 if !h.prepare(w,r){return}
 project:=r.PostFormValue("project")
 service:=r.PostFormValue("service")
 if strings.TrimSpace(project)!=project||strings.TrimSpace(service)!=service||
    project==""||service==""||len(project)>128||len(service)>128{
  http.Error(w,"Invalid service identity",http.StatusBadRequest);return
 }
 var target *string
 switch r.PostFormValue("action"){
 case "set":
  image:=r.PostFormValue("image")
  target=&image
 case "reset":
  // Reset needs no user-supplied image; use the last verified Docker image
  // and store-level recovery guard for a stale repository override.
 default:
  http.Error(w,"Invalid watched-image action",http.StatusBadRequest);return
 }
 if err:=h.Backend.SetServiceWatchOverride(r.Context(),project,service,target);err!=nil{
  http.Error(w,"Watched image could not be changed",http.StatusBadRequest);return
 }
 dest:="/services/"+url.PathEscape(project)+"/"+url.PathEscape(service)
 http.Redirect(w,r,dest+"?saved=watch#monitoring-controls",http.StatusSeeOther)
}
