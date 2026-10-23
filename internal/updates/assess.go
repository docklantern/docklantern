// Package updates assesses remote OCI manifest identity using conservative evidence.
// Container ImageID (sha256 config ID) is not an OCI manifest digest.
package updates

import (
 "strings"

 "github.com/docklantern/docklantern/internal/registry"
 "github.com/docklantern/docklantern/internal/store"
)
type Assessment struct {
 Project string `json:"project"`
 Service string `json:"service"`
 Image string `json:"image"`
 Platform string `json:"platform"`
 State string `json:"state"`
 Reason string `json:"reason"`
 RemoteDigest string `json:"remote_digest,omitempty"`
 DeployedDigest string `json:"deployed_digest,omitempty"`
 ContainerCount int `json:"container_count"`
}
func Assess(projects []store.Registration,containers []store.Container,policies []store.MonitorPolicy,checks []store.RegistryCheck)[]Assessment{
 registered:=make(map[string]map[string]bool)
 for _,p:=range projects{
  services:=make(map[string]bool)
  for _,s:=range p.Services{services[s]=true}
  registered[p.Name]=services
 }
 remote:=make(map[string]store.RegistryCheck,len(checks))
 for _,c:=range checks{remote[c.Image+"|"+c.Platform]=c}
 result:=make([]Assessment,0,len(policies))
 for _,p:=range policies{
  a:=Assessment{Project:p.Project,Service:p.Service,Image:p.Image,Platform:p.Platform,State:"unknown",Reason:"No matching immutable deployed manifest reference available"}
  if !registered[p.Project][p.Service]{a.State="unregistered";a.Reason="Service is not currently registered";result=append(result,a);continue}
  c,ok:=remote[p.Image+"|"+p.Platform]
  if !ok{a.State="not_checked";a.Reason="No successful remote registry check";result=append(result,a);continue}
  a.RemoteDigest=c.PlatformDigest
  if p.Paused{a.State="paused";a.Reason="Monitoring policy is paused";result=append(result,a);continue}
  var replicas []store.Container
  for _,item:=range containers {
   if item.Project==p.Project&&item.Service==p.Service {replicas=append(replicas,item)}
  }
  a.ContainerCount=len(replicas)
  if len(replicas)==0{a.State="not_running";a.Reason="No container recorded in last inventory scan";result=append(result,a);continue}
  // A multi-replica service can only have a definite state when all replicas
  // supply identical, verifiable pinned manifest references.
  common:=""
  valid:=true
  for _,replica:=range replicas{
   digest,good:=deployedManifestDigest(replica,p.Image,p.Platform)
   if !good|| (common!=""&&common!=digest){valid=false;break}
   common=digest
  }
  if !valid{
   a.State="unknown"
   a.Reason="Missing, platform-mismatched, or ambiguous image provenance; image configuration IDs are not manifest digests"
   result=append(result,a);continue
  }
  a.DeployedDigest=common
  if common==c.PlatformDigest||common==c.IndexDigest{
   a.State="current"
   a.Reason="Pinned container reference matches the last checked remote manifest or index"
  }else{
   a.State="different"
   a.Reason="Pinned deployed manifest differs from the last checked remote tag; direction of change is not established"
  }
  result=append(result,a)
 }
 return result
}
func pinnedDigest(containerImage,watchedImage string)(string,bool){
 before,after,found:=strings.Cut(containerImage,"@")
 if !found||!strings.HasPrefix(after,"sha256:")||len(after)!=71 {return "",false}
 for _,r:=range after[7:]{if !strings.ContainsRune("0123456789abcdef",r){return "",false}}
 containerRef,err:=registry.Parse(before);if err!=nil{return "",false}
 watchedRef,err:=registry.Parse(watchedImage);if err!=nil{return "",false}
 if containerRef.Registry!=watchedRef.Registry||containerRef.Repository!=watchedRef.Repository{return "",false}
 return after,true
}

// deployedManifestDigest requires either a pinned original container reference
// or local image inspect RepoDigests bound to the exact running config ID during
// the discovery scan. Multiple digests for the same repository are ambiguous.
func deployedManifestDigest(c store.Container,watched,platform string)(string,bool){
 pinned,pinnedOK:=pinnedDigest(c.Image,watched)
 // If metadata exists, its platform must match the monitored platform.
 if len(c.RepoDigests)>0 {
  actual:=c.ImageOS+"/"+c.ImageArchitecture
  if c.ImageVariant!=""{actual+="/"+c.ImageVariant}
  if actual!=platform||c.ImageOS==""||c.ImageArchitecture==""{return "",false}
  unique:=""
  for _,ref:=range c.RepoDigests{
   digest,ok:=pinnedDigest(ref,watched)
   if !ok{continue}
   if unique!=""&&unique!=digest{return "",false}
   unique=digest
  }
  if unique!=""{
   if pinnedOK&&pinned!=unique{return "",false}
   return unique,true
  }
  // Metadata cannot establish manifest identity for the watched repository.
  // A pinned original reference is still direct evidence, subject to platform.
  if pinnedOK{return pinned,true}
  return "",false
 }
 return pinned,pinnedOK
}
