// Package monitor schedules read-only OCI image manifest checks.
package monitor

import (
 "context"
 "errors"
 "fmt"
 "time"
 "github.com/docklantern/docklantern/internal/registry"
 "github.com/docklantern/docklantern/internal/store"
)

// Resolver may be backed by OCI registries; tests can use a deterministic fake.
type Resolver interface { Check(context.Context,string,string)(registry.Result,error) }
type Storage interface{
 DueMonitorPolicies(context.Context,time.Time,int)([]store.MonitorPolicy,error)
 RecordRegistryCheck(context.Context,string,string,string,string)(store.RegistryCheck,error)
 FinishMonitorCheck(context.Context,store.MonitorPolicy,time.Time,error)error
}
type Scheduler struct{
 Storage Storage
 Resolver Resolver
 PollEvery time.Duration
 CheckTimeout time.Duration
 BatchSize int
 Now func()time.Time
 Reporter interface{RecordWorkerRun(context.Context,string,time.Time,int,error)error}
}
func(s Scheduler) validate()error{
 if s.Storage==nil||s.Resolver==nil{return errors.New("monitor requires storage and resolver")}
 if s.PollEvery<time.Second||s.CheckTimeout<time.Second||s.BatchSize<1||s.BatchSize>100{return errors.New("invalid monitor limits")}
 return nil
}
func(s Scheduler) now()time.Time{if s.Now!=nil{return s.Now()};return time.Now()}
// RunOnce is deliberately sequential so the same process never checks a service
// concurrently, with bounded batch size and a per-registry-request timeout.
func(s Scheduler) RunOnce(ctx context.Context)(int,error){
 if err:=s.validate();err!=nil{return 0,err}
 policies,err:=s.Storage.DueMonitorPolicies(ctx,s.now(),s.BatchSize);if err!=nil{return 0,err}
 var first error
 for _,p:=range policies{
  if err:=ctx.Err();err!=nil{return len(policies),err}
  checkCtx,cancel:=context.WithTimeout(ctx,s.CheckTimeout)
  result,checkErr:=s.Resolver.Check(checkCtx,p.Image,p.Platform)
  cancel()
  if checkErr==nil{
   if result.Image!=p.Image||result.Platform!=p.Platform||result.IndexDigest==""||result.PlatformDigest==""{
    checkErr=errors.New("registry identity mismatch")
   }else{
    _,checkErr=s.Storage.RecordRegistryCheck(ctx,result.Image,result.Platform,result.IndexDigest,result.PlatformDigest)
   }
  }
  // No persistence of secrets; HTTP errors must not include credentials.
  if err:=s.Storage.FinishMonitorCheck(ctx,p,s.now(),checkErr);err!=nil{return len(policies),fmt.Errorf("save monitoring attempt: %w",err)}
  if checkErr!=nil&&first==nil{first=checkErr}
 }
 return len(policies),first
}
func(s Scheduler) Run(ctx context.Context)error{
 if err:=s.validate();err!=nil{return err}
 ticker:=time.NewTicker(s.PollEvery);defer ticker.Stop()
 // Check due jobs immediately on startup. Failures are recorded; continue polling.
 for {
  count,err:=s.RunOnce(ctx)
  if ctx.Err()!=nil{return nil}
  if s.Reporter!=nil{
   if e:=s.Reporter.RecordWorkerRun(ctx,"registry_monitor",s.now(),count,err);e!=nil{return fmt.Errorf("monitor heartbeat: %w",e)}
  }
  // Individual registry errors have already been recorded and backed off.
  // Infrastructure errors stop the worker so the supervisor can report failure.
  select{case <-ctx.Done():return nil;case <-ticker.C:}
 }
}
