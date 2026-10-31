package settings

import (
    "net/http"
    "net/url"
    "strings"
    "time"

    "github.com/docklantern/docklantern/internal/presentation"
    "github.com/docklantern/docklantern/internal/store"
)

// Check Now is a CSRF-protected scheduling hint. These HTTP handlers never
// resolve registry manifests, pull Docker images or run Compose operations.
// The existing background worker will process due policies on its next tick.
func(h Handler)checkNowService(w http.ResponseWriter,r *http.Request){
    if !h.prepare(w,r){return}
    project:=strings.TrimSpace(r.PostFormValue("project"))
    service:=strings.TrimSpace(r.PostFormValue("service"))
    if project==""||service==""||len(project)>128||len(service)>128{
        http.Error(w,"Invalid service identity",http.StatusBadRequest);return
    }
    outcome,err:=h.Backend.RequestServiceCheck(r.Context(),project,service,time.Now().UTC())
    if err!=nil{http.Error(w,"Check request unavailable",http.StatusInternalServerError);return}
    if outcome.State==store.CheckNotEligible{
        http.Error(w,"Service is not eligible for a registry check",http.StatusConflict);return
    }
    dest:="/services/"+url.PathEscape(project)+"/"+url.PathEscape(service)
    http.Redirect(w,r,dest+"?check="+url.QueryEscape(outcome.State)+"#monitoring-controls",http.StatusSeeOther)
}

func(h Handler)checkNowImage(w http.ResponseWriter,r *http.Request){
    if !h.prepare(w,r){return}
    encoded:=r.PostFormValue("image_id")
    identity,err:=presentation.ParseImageID(encoded)
    if err!=nil{http.Error(w,"Invalid image identity",http.StatusBadRequest);return}
    outcome,err:=h.Backend.RequestImageCheck(r.Context(),identity.Reference,identity.Platform,time.Now().UTC())
    if err!=nil{http.Error(w,"Check request unavailable",http.StatusInternalServerError);return}
    if outcome.State==store.CheckNotEligible{
        http.Error(w,"Image has no eligible registry watches",http.StatusConflict);return
    }
    http.Redirect(w,r,"/images/"+url.PathEscape(encoded)+"?check="+url.QueryEscape(outcome.State),http.StatusSeeOther)
}
