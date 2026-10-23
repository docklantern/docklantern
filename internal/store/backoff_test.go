package store

import (
 "context"
 "errors"
 "path/filepath"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/registry"
)

func TestPersistentBackoffAndRecovery(t *testing.T){
 path:=filepath.Join(t.TempDir(),"monitor.db")
 ctx:=context.Background()
 s,err:=Open(path);if err!=nil{t.Fatal(err)}
 reg:=Registration{Name:"test",ProjectDir:"/tmp/test",Files:[]string{"/tmp/test/compose.yaml"},Services:[]string{"web"}}
 if err=s.Register(ctx,reg);err!=nil{t.Fatal(err)}
 p:=MonitorPolicy{Project:"test",Service:"web",Image:"nginx:latest",Platform:"linux/amd64",IntervalSeconds:60}
 if err=s.SetMonitorPolicy(ctx,p);err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Truncate(time.Second)
 due,err:=s.DueMonitorPolicies(ctx,now.Add(time.Second),10);if err!=nil||len(due)!=1{t.Fatalf("%+v %v",due,err)}
 if err=s.FinishMonitorCheck(ctx,due[0],now,errors.New("transient unavailable"));err!=nil{t.Fatal(err)}
 policies,err:=s.MonitorPolicies(ctx);if err!=nil{t.Fatal(err)}
 first:=policies[0]
 if first.ConsecutiveFailures!=1{t.Fatalf("%+v",first)}
 expect,_:=time.Parse(time.RFC3339,first.NextCheckAt)
 if expect.Sub(now)!=120*time.Second{t.Fatalf("first backoff %v",expect.Sub(now))}
 _=s.Close()
 s,err=Open(path);if err!=nil{t.Fatal(err)};defer s.Close()
 due,err=s.DueMonitorPolicies(ctx,expect.Add(time.Second),10);if err!=nil||len(due)!=1{t.Fatalf("restart %+v %v",due,err)}
 secondNow:=expect.Add(time.Second)
 retry:=&registry.RetryableError{Status:429,RetryAfter:20*time.Minute}
 if err=s.FinishMonitorCheck(ctx,due[0],secondNow,retry);err!=nil{t.Fatal(err)}
 policies,err=s.MonitorPolicies(ctx);if err!=nil{t.Fatal(err)}
 second:=policies[0]
 if second.ConsecutiveFailures!=2{t.Fatalf("%+v",second)}
 retryAt,_:=time.Parse(time.RFC3339,second.NextCheckAt)
 if retryAt.Sub(secondNow)!=20*time.Minute{t.Fatalf("Retry-After ignored: %v",retryAt.Sub(secondNow))}
 due,err=s.DueMonitorPolicies(ctx,retryAt.Add(time.Second),10);if err!=nil||len(due)!=1{t.Fatalf("%+v %v",due,err)}
 if err=s.FinishMonitorCheck(ctx,due[0],retryAt.Add(time.Second),nil);err!=nil{t.Fatal(err)}
 policies,err=s.MonitorPolicies(ctx);if err!=nil{t.Fatal(err)}
 if policies[0].ConsecutiveFailures!=0||policies[0].LastError!=""{t.Fatalf("success did not reset state: %+v",policies[0])}
}
func TestBackoffDoesNotOverwriteChangedPolicy(t *testing.T){
 ctx:=context.Background();s,err:=Open(filepath.Join(t.TempDir(),"db.sqlite"));if err!=nil{t.Fatal(err)};defer s.Close()
 _=s.Register(ctx,Registration{Name:"app",ProjectDir:"/tmp/app",Files:[]string{"/tmp/app/compose.yaml"},Services:[]string{"web"}})
 p:=MonitorPolicy{Project:"app",Service:"web",Image:"nginx:latest",Platform:"linux/amd64",IntervalSeconds:60}
 _=s.SetMonitorPolicy(ctx,p)
 due,_:=s.DueMonitorPolicies(ctx,time.Now().Add(time.Second),10)
 p.IntervalSeconds=180
 _=s.SetMonitorPolicy(ctx,p)
 if err=s.FinishMonitorCheck(ctx,due[0],time.Now(),errors.New("stale attempt"));err!=nil{t.Fatal(err)}
 policies,_:=s.MonitorPolicies(ctx)
 if policies[0].ConsecutiveFailures!=0||policies[0].LastError!=""{t.Fatalf("overwrote edited policy: %+v",policies[0])}
}
