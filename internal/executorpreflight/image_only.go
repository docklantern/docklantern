// Package executorpreflight evaluates a strict, normalized, image-only
// Compose configuration change. It does not invoke Docker/Compose or grant
// deployment authorization. Real source, OCI and actual-state checks remain
// independent mandatory gates.
package executorpreflight

import (
 "bytes"
 "encoding/json"
 "errors"
 "io"
 "reflect"
 "strings"

 "github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrNotImageOnly = errors.New("unsupported or non-image-only Compose change")

const MaxConfigBytes = 512 << 10

// CheckImageOnly checks two independently normalized Compose config JSON
// objects. An eligible change modifies exactly one explicitly registered
// service's image field. Every other field (including every sibling service,
// network, volume, port, mount, environment value and label) must be equal.
//
// This pure comparison is NOT sufficient to approve deployment: a trusted
// host preflight must pin source files/env, reject unsupported Compose
// constructs, independently resolve OCI platform manifests, and check actual
// Docker state again immediately before side effects.
func CheckImageOnly(before,after []byte,reg executorregistration.Registration,service,repository,digest string) error {
 if len(before)==0||len(after)==0||len(before)>MaxConfigBytes||len(after)>MaxConfigBytes||
  reg.Validate()!=nil||!reg.AllowsTarget(service,repository,executorregistration.Platform{OS:"linux",Architecture:"amd64"})||
  !validDigest(digest) {
  return ErrNotImageOnly
 }
 a,err:=parseNormalized(before)
 if err!=nil{return ErrNotImageOnly}
 b,err:=parseNormalized(after)
 if err!=nil||ValidateNormalizedFeatureSubset(a)!=nil||ValidateNormalizedFeatureSubset(b)!=nil{return ErrNotImageOnly}
 beforeServices,ok:=a["services"].(map[string]any)
 if !ok{return ErrNotImageOnly}
 afterServices,ok:=b["services"].(map[string]any)
 if !ok||len(beforeServices)!=len(afterServices){return ErrNotImageOnly}
 // Every sibling is in the Compose project, not merely the selected service.
 // A host-sensitive construct in an untouched service makes execution unsafe.
 for name,serviceConfig:=range beforeServices {
  candidateConfig,exists:=afterServices[name]
  oldService,oldOK:=serviceConfig.(map[string]any)
  nextService,newOK:=candidateConfig.(map[string]any)
  if !exists||!oldOK||!newOK||
   unsupportedService(oldService)||unsupportedService(nextService){
   return ErrNotImageOnly
  }
 }
 old,ok:=beforeServices[service].(map[string]any)
 if !ok{return ErrNotImageOnly}
 newer,ok:=afterServices[service].(map[string]any)
 if !ok{return ErrNotImageOnly}
 // This initial beta.6 feature subset is intentionally conservative:
 // an independently checked, prebuilt single-container service only.
 if unsupportedService(old)||unsupportedService(newer){return ErrNotImageOnly}
 oldImage,ok:=old["image"].(string)
 if !ok||oldImage==""{return ErrNotImageOnly}
 newImage,ok:=newer["image"].(string)
 if !ok||newImage!=repository+"@"+digest||oldImage==newImage{return ErrNotImageOnly}

 // Work on the decoded 'after' config only. Put its selected image back to
 // the observed 'before' value, then require structural equality of the
 // *entire* configuration. No untracked exclusions or semantic shortcuts.
 newer["image"]=oldImage
 if !reflect.DeepEqual(a,b){return ErrNotImageOnly}
 return nil
}

func validDigest(d string)bool {
 if len(d)!=71||!strings.HasPrefix(d,"sha256:"){return false}
 for _,c:=range d[7:]{if (c<'0'||c>'9')&&(c<'a'||c>'f'){return false}}
 return true
}

func unsupportedService(s map[string]any)bool{
 for _,key:=range []string{"build","privileged","develop","deploy","hooks","lifecycle","extends","profiles",
  "env_file","configs","secrets","credential_spec", "provider", "post_start","pre_stop",
  "network_mode","pid","ipc","cgroup","cgroup_parent","userns_mode",
  "devices","device_cgroup_rules","cap_add","security_opt","sysctls",
  "volumes_from","links","external_links","uts","isolation","runtime"}{
  if _,exists:=s[key];exists{return true}
 }
 if _,ok:=s["image"].(string);!ok{return true}
 if n,ok:=s["init"].(bool);ok&&n{return true}
 if _,exists:=s["depends_on"];exists{return true}
 // Linux docker socket bind/mount cannot be allowed in beta.6 executor.
 if mounts,ok:=s["volumes"].([]any);ok {
  for _,value:=range mounts {
   if str,ok:=value.(string);ok&&unsafeHostMount(str){return true}
   if m,ok:=value.(map[string]any);ok {
    source,_:=m["source"].(string)
    target,_:=m["target"].(string)
    if unsafeHostMount(source)||unsafeHostMount(target){return true}
   }
  }
 }
 return false
}

func unsafeHostMount(s string)bool{
 // Docker socket and host operating-system control surfaces cannot appear
 // in a single-service image update. Reject both short- and long-form binds.
 for _,part:=range strings.Split(s,":"){
  clean:=strings.TrimSuffix(part,"/")
  if strings.Contains(clean,"docker.sock")||
   clean=="/proc"||strings.HasPrefix(clean,"/proc/")||
   clean=="/sys"||strings.HasPrefix(clean,"/sys/")||
   clean=="/run"||strings.HasPrefix(clean,"/run/")||
   clean=="/var/run"||strings.HasPrefix(clean,"/var/run/")||
   clean=="/dev"||strings.HasPrefix(clean,"/dev/"){
   return true
  }
 }
 return false
}

func parseNormalized(data []byte)(map[string]any,error){
 var decoded map[string]any
 dec:=json.NewDecoder(bytes.NewReader(data))
 dec.UseNumber()
 if e:=dec.Decode(&decoded);e!=nil||decoded==nil{return nil,ErrNotImageOnly}
 var tail any
 if e:=dec.Decode(&tail);e!=io.EOF{return nil,ErrNotImageOnly}
 return decoded,nil
}
