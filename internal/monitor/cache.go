package monitor

import (
 "context"
 "errors"
 "fmt"
 "sync"
 "time"

 "github.com/docklantern/docklantern/internal/registry"
)

// cacheScopeProvider must identify the effective authorization scope of an OCI
// request, without disclosing credentials. Resolvers without it are not cached:
// a custom resolver could otherwise switch credentials between requests.
type cacheScopeProvider interface { CacheScope(image string) string }

type imageKey struct { image, platform, scope string }
type hostKey struct { registry, scope string }
type cacheEntry struct {
 result registry.Result
 err error
 expires time.Time
}
type registryCooldown struct{
 until time.Time
 status int
}

// CachingResolver shares a recently resolved image/platform observation
// among services while keeping per-service schedule, backoff and events in
// the Store. Locking includes network I/O intentionally: at most one request
// can be in flight per scheduler, appropriate for small VPS instances.
// The cache is in-process only; it never stores registry tokens or secrets.
type CachingResolver struct{
 Base Resolver
 mu sync.Mutex
 entries map[imageKey]cacheEntry
 cooldowns map[hostKey]registryCooldown
 now func()time.Time
}

// NewCachingResolver uses a 45-second reuse window and 128 bounded entries.
// The scheduler retains its independent per-check network timeout.
func NewCachingResolver(base Resolver)*CachingResolver{
 return &CachingResolver{Base:base,entries:make(map[imageKey]cacheEntry),
  cooldowns:make(map[hostKey]registryCooldown),now:time.Now}
}
const cacheTTL=45*time.Second
const maxCachedImages=128
const maxHostCooldowns=64

func(r *CachingResolver) Check(ctx context.Context,image,platform string)(registry.Result,error){
 if r==nil||r.Base==nil{return registry.Result{},errors.New("registry resolver unavailable")}
 if err:=ctx.Err();err!=nil{return registry.Result{},err}
 ref,err:=registry.Parse(image)
 if err!=nil{return registry.Result{},err}
 scoped,ok:=r.Base.(cacheScopeProvider)
 if !ok {
  // Unknown credential scope: never reuse a result across requests.
  return r.Base.Check(ctx,image,platform)
 }
 scope:=scoped.CacheScope(ref.Key())
 if scope==""{return r.Base.Check(ctx,image,platform)}
 key:=imageKey{ref.Key(),platform,scope}
 host:=hostKey{ref.Registry,scope}

 r.mu.Lock()
 defer r.mu.Unlock()
 now:=r.now()
 r.prune(now)
 if entry,ok:=r.entries[key];ok&&now.Before(entry.expires){
  if err:=ctx.Err();err!=nil{return registry.Result{},err}
  return entry.result,entry.err
 }
 if hold,ok:=r.cooldowns[host];ok&&now.Before(hold.until){
  if err:=ctx.Err();err!=nil{return registry.Result{},err}
  // Return remaining delay, not the full original Retry-After duration.
  return registry.Result{},&registry.RetryableError{Status:hold.status,RetryAfter:hold.until.Sub(now)}
 }

 if err:=ctx.Err();err!=nil{return registry.Result{},err}
 result,checkErr:=r.Base.Check(ctx,ref.Key(),platform)
 if checkErr==nil&&(result.Image!=ref.Key()||result.Platform!=platform||
  result.IndexDigest==""||result.PlatformDigest==""){
  checkErr=errors.New("registry identity mismatch")
 }
 // Canceled requests are not reusable.
 if ctx.Err()!=nil{return registry.Result{},ctx.Err()}
 now=r.now()
 if checkErr==nil{
  r.add(key,cacheEntry{result:result,expires:now.Add(cacheTTL)})
  return result,nil
 }
 var retry *registry.RetryableError
 if errors.As(checkErr,&retry){
  // A 429 without Retry-After still needs a minimum host cooldown to
  // prevent a burst of other image checks against the same registry.
  if retry.Status==429||retry.Status==503 {
   delay:=retry.RetryAfter
   if delay<=0&&retry.Status==429{delay=time.Minute}
   if delay>24*time.Hour{delay=24*time.Hour}
   if delay>0 {
    if len(r.cooldowns)>=maxHostCooldowns {r.evictCooldown()}
    r.cooldowns[host]=registryCooldown{until:now.Add(delay),status:retry.Status}
    // Preserve retry-after for each watched service's persistent backoff.
    return registry.Result{},&registry.RetryableError{Status:retry.Status,RetryAfter:delay}
   }
   r.add(key,cacheEntry{err:checkErr,expires:now.Add(cacheTTL)})
  }
 }
 // Unexpected failures, authorization failures and invalid OCI manifests
 // aren't cached. Their per-service backoff is managed by SQLite.
 return registry.Result{},checkErr
}
func(r *CachingResolver) add(key imageKey,entry cacheEntry){
 if len(r.entries)>=maxCachedImages{
  var oldest imageKey
  var when time.Time
  for k,e:=range r.entries{if when.IsZero()||e.expires.Before(when){oldest=k;when=e.expires}}
  delete(r.entries,oldest)
 }
 r.entries[key]=entry
}
func(r *CachingResolver) evictCooldown(){
 var oldest hostKey
 var when time.Time
 for k,c:=range r.cooldowns{if when.IsZero()||c.until.Before(when){oldest=k;when=c.until}}
 delete(r.cooldowns,oldest)
}
func(r *CachingResolver) prune(now time.Time){
 for k,e:=range r.entries{if !now.Before(e.expires){delete(r.entries,k)}}
 for k,c:=range r.cooldowns{if !now.Before(c.until){delete(r.cooldowns,k)}}
}
// Bounds exposes useful internal diagnostics to tests without exposing
// credential fingerprints or image keys.
func(r *CachingResolver) Bounds()(int,int,error){
 if r==nil{return 0,0,fmt.Errorf("resolver unavailable")}
 r.mu.Lock();defer r.mu.Unlock()
 return len(r.entries),len(r.cooldowns),nil
}
