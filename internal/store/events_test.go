package store

import(
 "context"
 "path/filepath"
 "strings"
 "testing"
 "time"
)
func TestDigestTransitionEventsAndPersistence(t *testing.T){
 ctx:=context.Background();path:=filepath.Join(t.TempDir(),"events.db")
 s,err:=Open(path);if err!=nil{t.Fatal(err)}
 a:="sha256:"+strings.Repeat("a",64);b:="sha256:"+strings.Repeat("b",64)
 write:=func(d string){
  t.Helper()
  if _,err:=s.RecordRegistryCheck(ctx,"ghcr.io/team/web:latest","linux/amd64",d,d);err!=nil{t.Fatal(err)}
 }
 write(a);write(a);write(b);write(b);write(a)
 events,err:=s.Events(ctx,50)
 if err!=nil||len(events)!=2||events[0].PreviousDigest!=b||events[0].Digest!=a||events[1].Digest!=b{t.Fatalf("%+v %v",events,err)}
 _=s.Close()
 s,err=Open(path);if err!=nil{t.Fatal(err)};defer s.Close()
 pending,err:=s.DueEvents(ctx,time.Now().UTC().Add(time.Minute),10)
 if err!=nil||len(pending)!=2{t.Fatalf("pending %+v %v",pending,err)}
 if err=s.CompleteDelivery(ctx,pending[0],time.Now().UTC(),false);err!=nil{t.Fatal(err)}
 if err=s.CompleteDelivery(ctx,pending[0],time.Now().UTC(),true);err==nil{t.Fatal("stale event receipt accepted")}
 events,err=s.Events(ctx,20);if err!=nil||events[1].DeliveryAttempts!=1||events[1].LastDeliveryError==""{t.Fatalf("%+v %v",events,err)}
 later,_:=s.DueEvents(ctx,time.Now().Add(2*time.Minute),10)
 if err=s.CompleteDelivery(ctx,later[0],time.Now().UTC(),true);err!=nil{t.Fatal(err)}
 events,err=s.Events(ctx,20);if err!=nil||events[1].DeliveryState!="delivered"{t.Fatalf("%+v %v",events,err)}
}
func TestFailureAndRecoveryEventsOnlyOnce(t *testing.T){
 ctx:=context.Background();s,err:=Open(filepath.Join(t.TempDir(),"events.db"));if err!=nil{t.Fatal(err)};defer s.Close()
 if err=s.Register(ctx,Registration{Name:"app",ProjectDir:"/tmp/app",Files:[]string{"/tmp/app/compose.yaml"},Services:[]string{"web"}});err!=nil{t.Fatal(err)}
 p:=MonitorPolicy{Project:"app",Service:"web",Image:"nginx:latest",Platform:"linux/amd64",IntervalSeconds:60}
 if err=s.SetMonitorPolicy(ctx,p);err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Truncate(time.Second)
 for i:=0;i<3;i++{
  due,err:=s.DueMonitorPolicies(ctx,now.Add(time.Second),10)
  if err!=nil||len(due)!=1{t.Fatalf("%+v %v",due,err)}
  if err=s.FinishMonitorCheck(ctx,due[0],now,errTransient{});err!=nil{t.Fatal(err)}
  next,_:=s.MonitorPolicies(ctx);now,_=time.Parse(time.RFC3339,next[0].NextCheckAt)
 }
 due,err:=s.DueMonitorPolicies(ctx,now.Add(time.Second),10);if err!=nil||len(due)!=1{t.Fatal(err)}
 if err=s.FinishMonitorCheck(ctx,due[0],now,nil);err!=nil{t.Fatal(err)}
 events,err:=s.Events(ctx,20)
 if err!=nil||len(events)!=3||events[0].Kind!="registry_recovered"||events[1].Kind!="registry_outage_prolonged"||events[2].Kind!="registry_failure"{t.Fatalf("%+v %v",events,err)}
}
type errTransient struct{}
func(errTransient)Error()string{return "secret in upstream response must never be stored as event content"}
