// Package runtime evaluates local worker readiness without exposing stored metadata.
package runtime

import (
 "context"
 "errors"
 "fmt"
 "time"
 "github.com/docklantern/docklantern/internal/store"
)
type Reader interface{WorkerStatuses(context.Context)([]store.WorkerStatus,error)}
type Requirement struct{Name string;MaxAge time.Duration}
type Checker struct{
 Reader Reader
 Required []Requirement
 Now func()time.Time
}
func(c Checker)Ready(ctx context.Context)error{
 if c.Reader==nil{return errors.New("worker status store unavailable")}
 states,err:=c.Reader.WorkerStatuses(ctx);if err!=nil{return errors.New("worker status unavailable")}
 byName:=make(map[string]store.WorkerStatus,len(states))
 for _,s:=range states{byName[s.Name]=s}
 now:=time.Now().UTC();if c.Now!=nil{now=c.Now().UTC()}
 for _,r:=range c.Required{
  if r.MaxAge<=0{return errors.New("invalid worker readiness interval")}
  s,ok:=byName[r.Name]
  if !ok||s.State!="ok"||s.LastHeartbeatAt==""{return fmt.Errorf("worker %s not ready",r.Name)}
  at,e:=time.Parse(time.RFC3339Nano,s.LastHeartbeatAt)
  if e!=nil||at.After(now.Add(5*time.Second))||now.Sub(at)>r.MaxAge{return fmt.Errorf("worker %s heartbeat stale",r.Name)}
 }
 return nil
}
// Report asynchronously receives terminal failures from supervised workers.
// It allows the caller to shut down HTTP and exit nonzero rather than silently
// leaving a degraded UI running after background goroutines stop.
type Supervisor struct{failures chan error}
func NewSupervisor()*Supervisor{return &Supervisor{failures:make(chan error,8)}}
func(s *Supervisor)Start(ctx context.Context,name string,fn func(context.Context)error){
 go func(){
  var err error
  defer func(){
   if p:=recover();p!=nil{err=fmt.Errorf("worker %s panicked",name)}
   if ctx.Err()==nil{
    if err==nil{err=fmt.Errorf("worker %s stopped unexpectedly",name)}
    select{case s.failures<-fmt.Errorf("%s: %w",name,err):default:}
   }
  }()
  err=fn(ctx)
 }()
}
func(s *Supervisor)Failures()<-chan error{return s.failures}
