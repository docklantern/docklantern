package monitor

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "strings"
 "github.com/docklantern/docklantern/internal/registry"
)

// HostScopedResolver never sends one registry's secret to any other registry.
// Without an exact host binding it can only perform anonymous lookups.
type HostScopedResolver struct {
 RegistryHost string
 Username string
 Token string
}
func(r HostScopedResolver) Check(ctx context.Context,image,platform string)(registry.Result,error){
 ref,err:=registry.Parse(image);if err!=nil{return registry.Result{},err}
 client:=registry.Client{}
 if r.Username!=""||r.Token!=""{
  if r.Username==""||r.Token==""||r.RegistryHost=="" {return registry.Result{},errors.New("registry credential host, username and token required together")}
  host:=strings.ToLower(r.RegistryHost)
  if host==ref.Registry {client.Credentials=registry.Credentials{Username:r.Username,Password:r.Token}}
 }
 return client.Check(ctx,image,platform)
}

 // CacheScope returns an opaque fingerprint of the effective registry auth
 // scope. Anonymous and host-scoped credentialed requests cannot share cached
 // manifest observations or registry rate-limit state.
func(r HostScopedResolver) CacheScope(image string)string{
 ref,err:=registry.Parse(image);if err!=nil{return ""}
 if r.Username==""&&r.Token==""{return "anonymous"}
 if strings.ToLower(r.RegistryHost)!=ref.Registry{return "anonymous"}
 if r.Username==""||r.Token==""{return ""} // invalid tuple: no caching
 secretDigest:=sha256.Sum256([]byte(ref.Registry+"\x00"+r.Username+"\x00"+r.Token))
 return "auth:"+hex.EncodeToString(secretDigest[:])
}
