// Package registry performs read-only OCI Distribution manifest checks.
package registry

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "regexp"
 "strings"
 "time"
)
const accept = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"
const maxManifest = 8 << 20
var component = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
var tagRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
var digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type Reference struct { Registry string `json:"registry"`; Repository string `json:"repository"`; Tag string `json:"tag"` }
func Parse(name string)(Reference,error) {
 if len(name)==0 || len(name)>512 || strings.ContainsAny(name,"?#@\\ \t\n") { return Reference{}, errors.New("invalid image reference") }
 if strings.Contains(name,"@") { return Reference{},errors.New("digest-pinned input not supported in tag polling") }
 ref:=Reference{Registry:"registry-1.docker.io",Tag:"latest"}
 segments:=strings.Split(name,"/")
 if len(segments)>1&&(strings.ContainsAny(segments[0],".:")||segments[0]=="localhost") {
  ref.Registry=segments[0];segments=segments[1:]
 }
 if ref.Registry=="docker.io"||ref.Registry=="index.docker.io" {ref.Registry="registry-1.docker.io"}
 if len(segments)==0{return Reference{},errors.New("missing repository")}
 last:=segments[len(segments)-1]
 if i:=strings.LastIndex(last,":");i>=0 { ref.Tag=last[i+1:];segments[len(segments)-1]=last[:i] }
 if !tagRE.MatchString(ref.Tag) {return Reference{},errors.New("invalid tag")}
 if ref.Registry=="registry-1.docker.io"&&len(segments)==1 {segments=append([]string{"library"},segments...)}
 for _,s:=range segments {if !component.MatchString(s) {return Reference{},errors.New("invalid repository name")}}
 if len(segments)>20 {return Reference{},errors.New("repository too deep")}
 if strings.ContainsAny(ref.Registry,"@/?#")||strings.Contains(ref.Registry,"..")||strings.Contains(ref.Registry,"\\") {return Reference{},errors.New("invalid registry hostname")}
 if u,e:=url.Parse("https://"+ref.Registry);e!=nil||u.Host==""||u.Hostname()==""||u.User!=nil||u.Path!=""||u.RawQuery!="" {return Reference{},errors.New("invalid registry hostname")}
 ref.Repository=strings.Join(segments,"/")
 return ref,nil
}
func (r Reference) Key()string{return r.Registry+"/"+r.Repository+":"+r.Tag}

type Credentials struct {Username string; Password string}
type Result struct {
 Image string `json:"image"`
 IndexDigest string `json:"index_digest"`
 PlatformDigest string `json:"platform_digest,omitempty"`
 Platform string `json:"platform"`
 MediaType string `json:"media_type"`
}
type Client struct{ HTTP *http.Client; Credentials Credentials }
func (c Client) client()*http.Client {
 if c.HTTP!=nil{return c.HTTP}
 return &http.Client{Timeout:15*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
}
func (c Client) Check(ctx context.Context,image,platform string)(Result,error){
 r,e:=Parse(image);if e!=nil{return Result{},e}
 parts:=strings.Split(platform,"/")
 if len(parts)!=2 && len(parts)!=3 {return Result{},errors.New("platform must be os/architecture[/variant]")}
 for _,p:=range parts {if !component.MatchString(p) {return Result{},errors.New("invalid platform")}}
 root:=Result{Image:r.Key(),Platform:platform}
 body,typ,digest,err:=c.manifest(ctx,r,r.Tag);if err!=nil{return Result{},err}
 root.IndexDigest=digest;root.MediaType=typ
 var obj struct {
  MediaType string `json:"mediaType"`
  Manifests []struct {
   Digest string `json:"digest"`
   Platform struct{OS string `json:"os"`;Architecture string `json:"architecture"`;Variant string `json:"variant"`} `json:"platform"`
  } `json:"manifests"`
 }
 if err=json.Unmarshal(body,&obj);err!=nil {return Result{},errors.New("invalid registry manifest JSON")}
 if strings.Contains(typ,"manifest.list")||strings.Contains(typ,"image.index")||len(obj.Manifests)>0 {
  for _,m:=range obj.Manifests {
   id:=m.Platform.OS+"/"+m.Platform.Architecture
   if m.Platform.Variant!="" {id+="/"+m.Platform.Variant}
   if id!=platform {continue}
   if !digestRE.MatchString(m.Digest) {return Result{},errors.New("invalid platform digest")}
   // Verify that the index descriptor points to real content; do not trust metadata alone.
   _,_,child,e:=c.manifest(ctx,r,m.Digest);if e!=nil{return Result{},e}
   if child!=m.Digest {return Result{},errors.New("platform descriptor mismatch")}
   root.PlatformDigest=child
   return root,nil
  }
  return Result{},fmt.Errorf("no image manifest for platform %s",platform)
 }
 if !strings.Contains(typ,"image.manifest")&&!strings.Contains(typ,"manifest.v2"){return Result{},errors.New("unsupported image manifest media type")}
 // Single-platform manifests do not encode platform; only the caller can establish suitability.
 root.PlatformDigest=digest
 return root,nil
}
func (c Client) manifest(ctx context.Context,r Reference,ref string)([]byte,string,string,error){
 endpoint:="https://"+r.Registry+"/v2/"+r.Repository+"/manifests/"+ref
 fetch:=func(token string)(*http.Response,error){
  req,e:=http.NewRequestWithContext(ctx,http.MethodGet,endpoint,nil);if e!=nil{return nil,e}
  req.Header.Set("Accept",accept)
  if token!=""{req.Header.Set("Authorization","Bearer "+token)}else if c.Credentials.Username!="" {req.SetBasicAuth(c.Credentials.Username,c.Credentials.Password)}
  return c.client().Do(req)
 }
 resp,e:=fetch("");if e!=nil{return nil,"","",errors.New("registry request failed")}
 if resp.StatusCode==http.StatusUnauthorized {
  challenge:=resp.Header.Get("WWW-Authenticate");resp.Body.Close()
  token,e:=c.bearer(ctx,challenge,r);if e!=nil{return nil,"","",e}
  resp,e=fetch(token);if e!=nil{return nil,"","",errors.New("registry request failed")}
 }
 defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{return nil,"","",fmt.Errorf("registry returned HTTP %d",resp.StatusCode)}
 body,e:=io.ReadAll(io.LimitReader(resp.Body,maxManifest+1));if e!=nil||len(body)>maxManifest{return nil,"","",errors.New("manifest exceeds limit or failed to read")}
 sum:=sha256.Sum256(body);actual:="sha256:"+hex.EncodeToString(sum[:])
 header:=resp.Header.Get("Docker-Content-Digest")
 if header!=""&&header!=actual {return nil,"","",errors.New("registry digest does not match manifest bytes")}
 if strings.HasPrefix(ref,"sha256:")&&ref!=actual {return nil,"","",errors.New("requested digest did not match manifest bytes")}
 typ:=strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"),";")[0])
 return body,typ,actual,nil
}
var authParam=regexp.MustCompile(`([a-zA-Z_]+)="([^"]*)"`)
func (c Client) bearer(ctx context.Context,challenge string,r Reference)(string,error){
 if !strings.HasPrefix(strings.ToLower(challenge),"bearer ") {return "",errors.New("unsupported registry authentication challenge")}
 attrs:=map[string]string{}
 for _,m:=range authParam.FindAllStringSubmatch(challenge,-1){attrs[strings.ToLower(m[1])]=m[2]}
 realm,e:=url.Parse(attrs["realm"]);if e!=nil||realm.Scheme!="https"||realm.Host==""||realm.User!=nil||realm.Fragment!=""{return "",errors.New("unsafe registry auth realm")}
 if attrs["service"]==""||attrs["scope"]=="" {return "",errors.New("incomplete registry auth challenge")}
 if attrs["scope"]!="repository:"+r.Repository+":pull"{return "",errors.New("unexpected registry authorization scope")}
 q:=realm.Query();q.Set("service",attrs["service"]);q.Set("scope",attrs["scope"]);realm.RawQuery=q.Encode()
 req,e:=http.NewRequestWithContext(ctx,http.MethodGet,realm.String(),nil);if e!=nil{return "",e}
 if c.Credentials.Username!=""{req.SetBasicAuth(c.Credentials.Username,c.Credentials.Password)}
 resp,e:=c.client().Do(req);if e!=nil{return "",errors.New("registry token request failed")};defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{return "",fmt.Errorf("registry token endpoint returned HTTP %d",resp.StatusCode)}
 var payload struct{Token string `json:"token"`;AccessToken string `json:"access_token"`}
 if e=json.NewDecoder(io.LimitReader(resp.Body,64<<10)).Decode(&payload);e!=nil{return "",errors.New("invalid registry token response")}
 if payload.Token!="" {return payload.Token,nil}
 if payload.AccessToken!="" {return payload.AccessToken,nil}
 return "",errors.New("registry token missing")
}
