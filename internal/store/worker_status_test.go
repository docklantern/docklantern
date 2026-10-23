package store

import (
 "context"
 "errors"
 "path/filepath"
 "strings"
 "testing"
 "time"
)
func TestWorkerHealthPersistsAndRedacts(t *testing.T){
 path:=filepath.Join(t.TempDir(),"worker.sqlite")
 s,err:=Open(path);if err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 now:=time.Now().UTC().Truncate(time.Second)
 if err=s.RecordWorkerStart(ctx,"registry_monitor",now);err!=nil{t.Fatal(err)}
 if err=s.RecordWorkerRun(ctx,"registry_monitor",now.Add(time.Minute),3,nil);err!=nil{t.Fatal(err)}
 if err=s.RecordWorkerRun(ctx,"registry_monitor",now.Add(2*time.Minute),0,errors.New("secret bearer token"));err!=nil{t.Fatal(err)}
 states,err:=s.WorkerStatuses(ctx)
 if err!=nil||len(states)!=1||states[0].State!="error"||states[0].LastSuccessAt==""||strings.Contains(states[0].LastError,"secret"){t.Fatalf("%+v %v",states,err)}
 _=s.Close()
 reopened,err:=Open(path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 states,err=reopened.WorkerStatuses(ctx)
 if err!=nil||len(states)!=1||states[0].Name!="registry_monitor"{t.Fatalf("%+v %v",states,err)}
 if err=reopened.RecordWorkerRun(ctx,"registry_monitor",now.Add(3*time.Minute),1,nil);err!=nil{t.Fatal(err)}
 states,err=reopened.WorkerStatuses(ctx)
 if err!=nil||states[0].State!="ok"||states[0].LastError!=""{t.Fatalf("%+v %v",states,err)}
 if err=reopened.RecordWorkerRun(ctx,"unknown",now,0,nil);err==nil{t.Fatal("unknown worker accepted")}
}
