package monitor

import (
 "context"
 "errors"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/registry"
 "github.com/docklantern/docklantern/internal/store"
)

type scopedCountingResolver struct{
 scope string
 calls int
 results map[string]registry.Result
 errorFor map[string]error
}
func(f *scopedCountingResolver) CacheScope(string)string{return f.scope}
func(f *scopedCountingResolver) Check(_ context.Context,image,platform string)(registry.Result,error){
 f.calls++
 if e:=f.errorFor[image+"|"+platform];e!=nil{return registry.Result{},e}
 if r,ok:=f.results[image+"|"+platform];ok{return r,nil}
 return registry.Result{Image:image,Platform:platform,IndexDigest:"sha256:index",PlatformDigest:"sha256:manifest"},nil
}
type unscopedResolver struct{calls int}
func(f *unscopedResolver) Check(_ context.Context,image,platform string)(registry.Result,error){
 f.calls++;return registry.Result{Image:image,Platform:platform,IndexDigest:"sha256:i",PlatformDigest:"sha256:m"},nil
}

func TestCacheSharesSameImagePlatformAndExpires(t *testing.T){
 underlying:=&scopedCountingResolver{scope:"public"}
 cache:=NewCachingResolver(underlying)
 now:=time.Date(2026,10,26,2,0,0,0,time.UTC)
 cache.now=func()time.Time{return now}
 for i:=0;i<5;i++{
  res,err:=cache.Check(context.Background(),"docker.io/library/nginx:latest","linux/amd64")
  if err!=nil||res.PlatformDigest!="sha256:manifest"{t.Fatalf("bad cached result: %+v %v",res,err)}
 }
 if underlying.calls!=1{t.Fatalf("expected one OCI lookup, got %d",underlying.calls)}
 // Canonical aliases share a key; platforms, tags and hosts do not.
 _,_ =cache.Check(context.Background(),"nginx:latest","linux/amd64")
 if underlying.calls!=1{t.Fatal("alias unexpectedly missed cache")}
 for _,tc:=range []struct{image,platform string}{
  {"nginx:latest","linux/arm64"},
  {"nginx:1.27","linux/amd64"},
  {"ghcr.io/example/nginx:latest","linux/amd64"},
  {"nginx:latest","linux/amd64/v3"},
 } {
  _,err:=cache.Check(context.Background(),tc.image,tc.platform)
  if err!=nil{t.Fatal(err)}
 }
 if underlying.calls!=5{t.Fatalf("different identities shared an observation: %d",underlying.calls)}
 now=now.Add(46*time.Second)
 _,err:=cache.Check(context.Background(),"nginx:latest","linux/amd64")
 if err!=nil||underlying.calls!=6{t.Fatalf("TTL did not expire: %d %v",underlying.calls,err)}
}

func TestCacheCredentialsAndAnonymousNeverShare(t *testing.T){
 public:=HostScopedResolver{}
 key:="ghcr.io/team/web:latest"
 a:=HostScopedResolver{RegistryHost:"ghcr.io",Username:"alice",Token:"secret-1"}
 b:=HostScopedResolver{RegistryHost:"ghcr.io",Username:"alice",Token:"secret-2"}
 wrongHost:=HostScopedResolver{RegistryHost:"example.com",Username:"alice",Token:"secret-1"}
 if a.CacheScope(key)==b.CacheScope(key)||a.CacheScope(key)==public.CacheScope(key){
  t.Fatal("different registry credentials share a cache scope")
 }
 if a.CacheScope("docker.io/library/nginx:latest")!="anonymous"{
  t.Fatal("credentials scoped to one host leaked into other registry scope")
 }
 if wrongHost.CacheScope(key)!="anonymous"{t.Fatal("wrong host credentials not isolated")}
 if strings.Contains(a.CacheScope(key),"secret-1"){t.Fatal("secret exposed in fingerprint")}
 invalid:=HostScopedResolver{RegistryHost:"ghcr.io",Username:"alice"}
 if invalid.CacheScope(key)!=""{t.Fatal("invalid credentials were eligible for caching")}
}

func TestCacheRespectsRetryAfterForEntireCredentialHost(t *testing.T){
 first:="ghcr.io/team/a:latest"
 second:="ghcr.io/team/b:latest"
 underlying:=&scopedCountingResolver{
  scope:"public",
  errorFor:map[string]error{first+"|linux/amd64":&registry.RetryableError{
   Status:429,RetryAfter:2*time.Minute,
  }},
 }
 cache:=NewCachingResolver(underlying)
 now:=time.Date(2026,10,26,2,0,0,0,time.UTC)
 cache.now=func()time.Time{return now}
 _,err:=cache.Check(context.Background(),first,"linux/amd64")
 var retry *registry.RetryableError
 if !errors.As(err,&retry)||retry.RetryAfter!=2*time.Minute{t.Fatalf("bad initial retry: %v",err)}
 now=now.Add(30*time.Second)
 _,err=cache.Check(context.Background(),second,"linux/amd64")
 if !errors.As(err,&retry)||retry.RetryAfter!=90*time.Second{t.Fatalf("missing host cooldown: %v",err)}
 if underlying.calls!=1{t.Fatal("rate-limited host made a second OCI request")}
 // Another registry is not throttled.
 _,err=cache.Check(context.Background(),"registry.example.com/team/a:latest","linux/amd64")
 if err!=nil||underlying.calls!=2{t.Fatalf("unrelated host blocked: %v calls=%d",err,underlying.calls)}
 now=now.Add(91*time.Second)
 _,err=cache.Check(context.Background(),second,"linux/amd64")
 if err!=nil||underlying.calls!=3{t.Fatalf("retry-after never expired: %v calls=%d",err,underlying.calls)}
}

func TestCacheDefaultCooldownAndBoundedMemory(t *testing.T){
 underlying:=&scopedCountingResolver{scope:"public",errorFor:map[string]error{
  "ghcr.io/team/blocked:latest|linux/amd64":&registry.RetryableError{Status:429},
 }}
 cache:=NewCachingResolver(underlying)
 now:=time.Date(2026,10,26,2,0,0,0,time.UTC)
 cache.now=func()time.Time{return now}
 _,err:=cache.Check(context.Background(),"ghcr.io/team/blocked:latest","linux/amd64")
 var retry *registry.RetryableError
 if !errors.As(err,&retry)||retry.RetryAfter<time.Minute{t.Fatalf("no default cooldown: %v",err)}
 for i:=0;i<300;i++{
  ref:="registry.example.com/team/app"+strings.Repeat("x",i/100)+string(rune('a'+i%26))+":tag"+strings.Repeat("v",i/26)
  // Use a unique OCI-valid reference without growing the tag beyond limits.
  ref="registry.example.com/team/app:tag"+itoa(i)
  _,err=cache.Check(context.Background(),ref,"linux/amd64")
  if err!=nil{t.Fatalf("bounded cache check %d: %v",i,err)}
 }
 entries,holds,err:=cache.Bounds()
 if err!=nil||entries>maxCachedImages||holds>maxHostCooldowns{
  t.Fatalf("unbounded cache memory: %d entries %d host cooldowns %v",entries,holds,err)
 }
}
func itoa(i int)string{
 if i==0{return "0"}
 s:=""
 for i>0{s=string(rune('0'+i%10))+s;i/=10}
 return s
}

func TestUnknownCredentialScopeCannotShareCache(t *testing.T){
 r:=&unscopedResolver{}
 cache:=NewCachingResolver(r)
 for i:=0;i<2;i++{
  if _,err:=cache.Check(context.Background(),"nginx:latest","linux/amd64");err!=nil{t.Fatal(err)}
 }
 if r.calls!=2{t.Fatal("unknown auth scope cached unsafely")}
}

func TestScheduledWatchersShareOCIRequestButKeepHistoryAndBackoff(t *testing.T){
 ctx:=context.Background()
 db,err:=store.Open(filepath.Join(t.TempDir(),"monitor.sqlite"))
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 image:="ghcr.io/example/shared:latest"
 for _,project:=range []string{"a","b","c"} {
  err=db.Register(ctx,store.Registration{
   Name:project,ProjectDir:"/srv/"+project,Files:[]string{"/srv/"+project+"/compose.yaml"},Services:[]string{"web"},
  })
  if err!=nil{t.Fatal(err)}
  err=db.SetMonitorPolicy(ctx,store.MonitorPolicy{
   Project:project,Service:"web",Image:image,Platform:"linux/amd64",IntervalSeconds:60,
   Paused:project=="c",
  })
  if err!=nil{t.Fatal(err)}
 }
 underlying:=&scopedCountingResolver{scope:"anonymous"}
 cache:=NewCachingResolver(underlying)
 now:=time.Now().UTC().Truncate(time.Second).Add(3*time.Second)
 cache.now=func()time.Time{return now}
 scheduler:=Scheduler{Storage:db,Resolver:cache,
  PollEvery:time.Second,CheckTimeout:time.Second,BatchSize:20,
  Now:func()time.Time{return now}}
 n,err:=scheduler.RunOnce(ctx)
 if err!=nil||n!=2||underlying.calls!=1{
  t.Fatalf("shared check n=%d requests=%d err=%v",n,underlying.calls,err)
 }
 policy,err:=db.MonitorPolicies(ctx)
 if err!=nil||len(policy)!=3{t.Fatal(err)}
 for _,p:=range policy{
  if p.Project!="c"&&(p.LastCheckedAt==""||p.Paused){t.Fatalf("missing per-service success: %+v",p)}
  if p.Project=="c"&&p.LastCheckedAt!=""{t.Fatal("paused watch checked")}
 }
 ev,err:=db.Events(ctx,20)
 if err!=nil||len(ev)!=0{t.Fatalf("first_seen emitted event: %+v",ev)}
 // Expire the cache and make both watches due at their independent schedules.
 now=now.Add(2*time.Minute)
 underlying.results=map[string]registry.Result{
  "ghcr.io/example/shared:latest|linux/amd64":{
   Image:image,Platform:"linux/amd64",IndexDigest:"sha256:index-2",PlatformDigest:"sha256:new"},
 }
 n,err=scheduler.RunOnce(ctx)
 if err!=nil||n!=2||underlying.calls!=2{t.Fatalf("changed n=%d calls=%d err=%v",n,underlying.calls,err)}
 ev,err=db.Events(ctx,20)
 if err!=nil||len(ev)!=1||ev[0].Kind!="remote_digest_changed"||ev[0].Project!=""{
  t.Fatalf("expected one image-scoped change event: %+v %v",ev,err)
 }
 // A third pass with the same digest is 'unchanged': zero extra events.
 now=now.Add(2*time.Minute)
 _,err=scheduler.RunOnce(ctx)
 if err!=nil||underlying.calls!=3{t.Fatalf("unchanged registry check: calls=%d err=%v",underlying.calls,err)}
 ev,_=db.Events(ctx,20)
 if len(ev)!=1{t.Fatalf("duplicate digest event: %+v",ev)}

 // A scoped 429 affects both due policies, but failure history remains
 // per service and the rate-limit state avoids a second network request.
 now=now.Add(2*time.Minute)
 underlying.errorFor=map[string]error{
  image+"|linux/amd64":&registry.RetryableError{Status:429,RetryAfter:10*time.Minute},
 }
 _,err=scheduler.RunOnce(ctx)
 if err==nil||underlying.calls!=4{t.Fatalf("expected one rate-limited request: %d %v",underlying.calls,err)}
 policy,_=db.MonitorPolicies(ctx)
 for _,p:=range policy{
  if p.Project!="c"&&(p.ConsecutiveFailures!=1||p.LastError==""||p.Paused){
   t.Fatalf("per-service retry state lost: %+v",p)
  }
 }
}
