package store

import (
 "context"
 "path/filepath"
 "strings"
 "testing"
 "time"
)
func TestNotificationPreferencesAndHistoryFilters(t *testing.T){
 ctx:=context.Background()
 s,err:=Open(filepath.Join(t.TempDir(),"events.db"));if err!=nil{t.Fatal(err)};defer s.Close()
 pref,err:=s.NotificationSettings(ctx)
 if err!=nil||pref.OutageFailures!=3||pref.RetentionDays!=30||!pref.DigestChanges{t.Fatalf("%+v %v",pref,err)}
 pref.DigestChanges=false
 pref.RetentionDays=5
 if err:=s.SetNotificationSettings(ctx,pref);err!=nil{t.Fatal(err)}
 _,err=s.RecordRegistryCheck(ctx,"ghcr.io/acme/api:latest","linux/amd64","sha256:a","sha256:a");if err!=nil{t.Fatal(err)}
 _,err=s.RecordRegistryCheck(ctx,"ghcr.io/acme/api:latest","linux/amd64","sha256:b","sha256:b");if err!=nil{t.Fatal(err)}
 events,err:=s.Events(ctx,100);if err!=nil||len(events)!=1{t.Fatalf("%+v %v",events,err)}
 due,err:=s.DueEvents(ctx,time.Now().Add(time.Second),10)
 if err!=nil||len(due)!=0{t.Fatalf("muted digest unexpectedly due: %+v %v",due,err)}
 pref.DigestChanges=true
 if err:=s.SetNotificationSettings(ctx,pref);err!=nil{t.Fatal(err)}
 due,err=s.DueEvents(ctx,time.Now().Add(time.Second),10)
 if err!=nil||len(due)!=1{t.Fatalf("enabled digest missing: %+v %v",due,err)}
 for _,term:=range []string{"api","acme","ghcr.io"}{
  result,err:=s.FilterEvents(ctx,EventFilter{Search:term,Limit:10});if err!=nil||len(result)!=1{t.Fatalf("%s %+v %v",term,result,err)}
 }
 result,err:=s.FilterEvents(ctx,EventFilter{Search:"%",Limit:10});if err!=nil||len(result)!=0{t.Fatalf("wildcard wasn't escaped %+v %v",result,err)}
 for _,f:=range []EventFilter{{Limit:0},{Limit:501},{Kind:"' OR 1=1 --",Limit:10},{DeliveryState:"invalid",Limit:10},{BeforeID:-1,Limit:10}}{
  if _,err:=s.FilterEvents(ctx,f);err==nil{t.Fatalf("accepted %+v",f)}
 }
 if err:=s.CompleteDelivery(ctx,due[0],time.Now().UTC(),true);err!=nil{t.Fatal(err)}
 _,err=s.db.ExecContext(ctx,"UPDATE monitor_events SET occurred_at=? WHERE id=?",time.Now().AddDate(0,0,-10).UTC().Format(time.RFC3339Nano),due[0].ID);if err!=nil{t.Fatal(err)}
 pruned,err:=s.PruneDeliveredEvents(ctx,time.Now().UTC())
 if err!=nil||pruned!=1{t.Fatalf("pruned %d %v",pruned,err)}
}
func TestOutageThresholdOnceAndRecovery(t *testing.T){
 ctx:=context.Background();s,err:=Open(filepath.Join(t.TempDir(),"outage.db"));if err!=nil{t.Fatal(err)};defer s.Close()
 _=s.Register(ctx,Registration{Name:"app",ProjectDir:"/tmp/app",Files:[]string{"/tmp/app/compose.yaml"},Services:[]string{"web"}})
 p:=MonitorPolicy{Project:"app",Service:"web",Image:"nginx:latest",Platform:"linux/amd64",IntervalSeconds:60}
 if err:=s.SetMonitorPolicy(ctx,p);err!=nil{t.Fatal(err)}
 pref,_:=s.NotificationSettings(ctx);pref.OutageFailures=2
 if err:=s.SetNotificationSettings(ctx,pref);err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Truncate(time.Second)
 for n:=0;n<4;n++{
  due,err:=s.DueMonitorPolicies(ctx,now.Add(time.Second),10);if err!=nil||len(due)!=1{t.Fatalf("attempt %d due %v %v",n,due,err)}
  if err=s.FinishMonitorCheck(ctx,due[0],now,errOutage{});err!=nil{t.Fatal(err)}
  policy,_:=s.MonitorPolicies(ctx);now,_=time.Parse(time.RFC3339,policy[0].NextCheckAt)
 }
 due,_:=s.DueMonitorPolicies(ctx,now.Add(time.Second),10)
 if err=s.FinishMonitorCheck(ctx,due[0],now,nil);err!=nil{t.Fatal(err)}
 events,err:=s.Events(ctx,30)
 if err!=nil||len(events)!=3||events[0].Kind!="registry_recovered"||events[1].Kind!="registry_outage_prolonged"||events[2].Kind!="registry_failure"{t.Fatalf("%+v %v",events,err)}
 if strings.Contains(events[1].Image,"secret"){t.Fatal("leaked secret")}
}
type errOutage struct{}
func(errOutage)Error()string{return "HTTP transient"}
func TestPendingRetentionProtected(t *testing.T){
 ctx:=context.Background();s,err:=Open(filepath.Join(t.TempDir(),"pending.db"));if err!=nil{t.Fatal(err)};defer s.Close()
 _,_=s.RecordRegistryCheck(ctx,"nginx:latest","linux/amd64","one","one")
 _,_=s.RecordRegistryCheck(ctx,"nginx:latest","linux/amd64","two","two")
 _,err=s.db.ExecContext(ctx,"UPDATE monitor_events SET occurred_at=?",time.Now().AddDate(0,0,-5000).UTC().Format(time.RFC3339Nano))
 if err!=nil{t.Fatal(err)}
 n,err:=s.PruneDeliveredEvents(ctx,time.Now())
 if err!=nil||n!=0{t.Fatalf("pending deleted %d %v",n,err)}
}
