package runtime
import(
 "context"
 "errors"
 "strings"
 "testing"
 "time"
 "github.com/docklantern/docklantern/internal/store"
)
type fakeReader struct{rows []store.WorkerStatus;err error}
func(f fakeReader)WorkerStatuses(context.Context)([]store.WorkerStatus,error){return f.rows,f.err}
func TestReadiness(t *testing.T){
 now:=time.Date(2026,10,23,10,0,0,0,time.UTC)
 base:=store.WorkerStatus{Name:"registry_monitor",State:"ok",LastHeartbeatAt:now.Add(-time.Minute).Format(time.RFC3339Nano)}
 cases:=[]struct{name string;row store.WorkerStatus;want bool}{
  {"fresh",base,true},
  {"stale",store.WorkerStatus{Name:base.Name,State:"ok",LastHeartbeatAt:now.Add(-10*time.Minute).Format(time.RFC3339Nano)},false},
  {"not started",store.WorkerStatus{Name:base.Name,State:"starting"},false},
  {"failed",store.WorkerStatus{Name:base.Name,State:"error",LastHeartbeatAt:base.LastHeartbeatAt},false},
  {"future",store.WorkerStatus{Name:base.Name,State:"ok",LastHeartbeatAt:now.Add(time.Minute).Format(time.RFC3339Nano)},false},
 }
 for _,tc:=range cases{
  checker:=Checker{Reader:fakeReader{rows:[]store.WorkerStatus{tc.row}},Now:func()time.Time{return now},Required:[]Requirement{{Name:base.Name,MaxAge:5*time.Minute}}}
  if got:=checker.Ready(context.Background())==nil;got!=tc.want{t.Fatalf("%s readiness=%v",tc.name,got)}
 }
 if err:=(Checker{Reader:fakeReader{err:errors.New("database credentials")}}).Ready(context.Background());err==nil||strings.Contains(err.Error(),"credentials"){t.Fatal("leaked database error")}
}
func TestSupervisedFailureAndPanic(t *testing.T){
 for _,fn:=range []func(context.Context)error{
  func(context.Context)error{return errors.New("failed")},
  func(context.Context)error{panic("sensitive value")},
  func(context.Context)error{return nil},
 }{
  ctx,cancel:=context.WithCancel(context.Background())
  s:=NewSupervisor()
  s.Start(ctx,"registry_monitor",fn)
  select{case err:=<-s.Failures():if err==nil||strings.Contains(err.Error(),"sensitive"){t.Fatalf("bad supervisor error: %v",err)}
  case <-time.After(2*time.Second):t.Fatal("worker exit not detected")}
  cancel()
 }
}
