package settings

import (
 "net/http"
 "net/url"
 "strings"
)

func(h Handler) saveDefaultInterval(w http.ResponseWriter,r *http.Request) {
 if !h.prepare(w,r){return}
 n,err:=parseInterval(r.PostFormValue("interval_seconds"))
 if err!=nil{http.Error(w,"Invalid default interval",http.StatusBadRequest);return}
 if err=h.Backend.SetDefaultMonitorInterval(r.Context(),n);err!=nil{
  http.Error(w,"Unable to save monitoring default",http.StatusBadRequest);return
 }
 http.Redirect(w,r,"/settings?saved=default#monitoring-defaults",http.StatusSeeOther)
}

// All operations require the same authentication, origin and CSRF checks as
// legacy Settings forms. Project/service are looked up as exact database
// identities; form input can never be interpreted as Docker/Compose commands.
func(h Handler) saveServiceSettings(w http.ResponseWriter,r *http.Request){
 if !h.prepare(w,r){return}
 project:=strings.TrimSpace(r.PostFormValue("project"))
 service:=strings.TrimSpace(r.PostFormValue("service"))
 if project==""||service==""||len(project)>128||len(service)>128{
  http.Error(w,"Invalid monitoring service",http.StatusBadRequest);return
 }
 var err error
 switch r.PostFormValue("action"){
 case "enable":
  err=h.Backend.SetServiceGUIEnrollment(r.Context(),project,service,true)
 case "disable":
  err=h.Backend.SetServiceGUIEnrollment(r.Context(),project,service,false)
 case "override":
  var n int
  n,err=parseInterval(r.PostFormValue("interval_seconds"))
  if err==nil{err=h.Backend.SetServiceIntervalOverride(r.Context(),project,service,&n)}
 case "reset":
  err=h.Backend.SetServiceIntervalOverride(r.Context(),project,service,nil)
 case "pause":
  err=h.Backend.SetServicePaused(r.Context(),project,service,true)
 case "resume":
  err=h.Backend.SetServicePaused(r.Context(),project,service,false)
 default:
  http.Error(w,"Invalid monitoring action",http.StatusBadRequest);return
 }
 if err!=nil{
  // Do not expose database errors, Docker metadata or host paths in HTTP.
  http.Error(w,"Monitoring settings could not be applied",http.StatusBadRequest);return
 }
 // The optional destination is an exact same-service route, never an arbitrary URL.
 // Missing/forged destinations retain the existing Settings behavior.
 target:="/settings?saved=service#service-monitoring"
 if requested:=r.PostFormValue("return_to"); requested!="" {
  expected:="/services/"+url.PathEscape(project)+"/"+url.PathEscape(service)
  if requested==expected{target=expected+"?saved=service#monitoring-controls"}
 }
 http.Redirect(w,r,target,http.StatusSeeOther)
}
