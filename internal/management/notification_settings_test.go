package management

import (
 "context"
 "net/url"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/store"
)
func TestAuthenticatedNotificationSettings(t *testing.T){
 h,db,_,guard:=setup(t)
 if got:=request(h,"GET","/events","",false).Code;got!=401{t.Fatalf("events public: %d",got)}
 if got:=request(h,"GET","/events","",true).Code;got!=200{t.Fatalf("events auth failed: %d",got)}
 if got:=request(h,"GET","/events?kind=bad","",true).Code;got!=400{t.Fatalf("invalid kind accepted: %d",got)}
 if got:=request(h,"GET","/api/v1/events/search?kind=bad","",true).Code;got!=400{t.Fatalf("invalid API kind accepted: %d",got)}
 form:=url.Values{"outage_failures":{"4"},"retention_days":{"14"},"digest_changes":{"on"},"prolonged_outage":{"on"}}
 if got:=request(h,"POST","/settings/notifications",form.Encode(),true).Code;got!=403{t.Fatalf("missing csrf accepted %d",got)}
 form.Set("csrf_token",guard.Token())
 if got:=request(h,"POST","/settings/notifications",form.Encode(),false).Code;got!=401{t.Fatalf("unauthorized accepted %d",got)}
 if got:=request(h,"POST","/settings/notifications",form.Encode(),true).Code;got!=303{t.Fatalf("settings update failed %d",got)}
 current,err:=db.NotificationSettings(context.Background())
 if err!=nil||current.OutageFailures!=4||current.RetentionDays!=30||current.RegistryFailures||!current.DigestChanges{t.Fatalf("%+v %v",current,err)}
 // The notification form can arrive from a stale tab; retention is now
 // saved by its own CSRF-protected handler and must not be reset here.
 newRetention:=url.Values{"csrf_token":{guard.Token()},"retention_days":{"17"}}
 if got:=request(h,"POST","/settings/retention",newRetention.Encode(),true).Code;got!=303{
  t.Fatalf("retention settings update failed: %d",got)
 }
 if got:=request(h,"POST","/settings/notifications",form.Encode(),true).Code;got!=303{
  t.Fatalf("stale notification form failed: %d",got)
 }
 current,err=db.NotificationSettings(context.Background())
 if err!=nil||current.RetentionDays!=17{
  t.Fatalf("notification form overwrote newer retention: %+v %v",current,err)
 }
 bad:=url.Values{"csrf_token":{guard.Token()},"outage_failures":{"1"},"retention_days":{"0"}}
 if got:=request(h,"POST","/settings/notifications",bad.Encode(),true).Code;got!=400{t.Fatalf("bad setting accepted %d",got)}
 prune:=url.Values{"csrf_token":{guard.Token()}}
 if got:=request(h,"POST","/settings/events/prune",prune.Encode(),true).Code;got!=400{t.Fatalf("prune without confirm accepted %d",got)}
 prune.Set("confirm","yes")
 if got:=request(h,"POST","/settings/events/prune",prune.Encode(),true).Code;got!=303{t.Fatalf("confirmed prune failed %d",got)}
 _=store.EventFilter{}
 // HTML must escape any search input.
 response:=request(h,"GET","/events?search=%3Cscript%3E","",true)
 if response.Code!=200||strings.Contains(response.Body.String(),"<script>"){t.Fatal("search input was not escaped")}
}
