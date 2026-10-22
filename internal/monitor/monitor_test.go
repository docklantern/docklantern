package monitor

import (
 "context"
 "errors"
 "testing"
 "time"
 "github.com/docklantern/docklantern/internal/registry"
 "github.com/docklantern/docklantern/internal/store"
)
type testDB struct {p []store.MonitorPolicy; calls int; recorded int; lastErr error}
func(d *testDB)DueMonitorPolicies(context.Context,time.Time,int)([]store.MonitorPolicy,error){d.calls++;return d.p,nil}
func(d *testDB)RecordRegistryCheck(context.Context,string,string,string,string)(store.RegistryCheck,error){d.recorded++;return store.RegistryCheck{},nil}
func(d *testDB)FinishMonitorCheck(_ context.Context,_ store.MonitorPolicy,_ time.Time,e error)error{d.lastErr=e;return nil}
type testResolver struct {res registry.Result; err error}
func(r testResolver)Check(context.Context,string,string)(registry.Result,error){return r.res,r.err}
func TestSuccessfulCheck(t *testing.T){
 p:=store.MonitorPolicy{Image:"registry-1.docker.io/library/nginx:latest",Platform:"linux/amd64",IntervalSeconds:90}
 d:=&testDB{p:[]store.MonitorPolicy{p}}
 s:=Scheduler{Storage:d,Resolver:testResolver{res:registry.Result{Image:p.Image,Platform:p.Platform,IndexDigest:"sha256:i",PlatformDigest:"sha256:c"}},PollEvery:time.Second,CheckTimeout:time.Second,BatchSize:5}
 n,e:=s.RunOnce(context.Background());if e!=nil||n!=1||d.recorded!=1||d.lastErr!=nil{t.Fatalf("n=%d e=%v record=%d last=%v",n,e,d.recorded,d.lastErr)}
}
func TestRemoteFailureDoesNotOverwriteDigest(t *testing.T){
 d:=&testDB{p:[]store.MonitorPolicy{{Image:"ghcr.io/demo/app:latest",Platform:"linux/amd64"}}}
 s:=Scheduler{Storage:d,Resolver:testResolver{err:errors.New("registry HTTP 429")},PollEvery:time.Second,CheckTimeout:time.Second,BatchSize:1}
 _,e:=s.RunOnce(context.Background());if e==nil||d.recorded!=0||d.lastErr==nil{t.Fatal("failed check incorrectly persisted digest")}
}
func TestIdentityMismatch(t *testing.T){
 d:=&testDB{p:[]store.MonitorPolicy{{Image:"ghcr.io/demo/app:latest",Platform:"linux/amd64"}}}
 s:=Scheduler{Storage:d,Resolver:testResolver{res:registry.Result{Image:"another",Platform:"linux/amd64",IndexDigest:"x",PlatformDigest:"y"}},PollEvery:time.Second,CheckTimeout:time.Second,BatchSize:1}
 if _,e:=s.RunOnce(context.Background());e==nil||d.recorded!=0{t.Fatal("accepted mismatched registry result")}
}
