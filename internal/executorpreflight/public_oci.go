package executorpreflight

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "mime"
 "net/http"
 "net/url"
 "strings"
 "time"

 "github.com/docklantern/docklantern/internal/executorregistration"
)

// Public Docker Hub only. All outbound hosts and auth scopes are fixed by
// trusted code, never sourced from an untrusted proposal or Docker config.
const dockerRegistryURL = "https://registry-1.docker.io"
const dockerTokenURL = "https://auth.docker.io/token"
const dockerTokenService = "registry.docker.io"
const publicOCITimeout = 15*time.Second
const maxPublicTokenBytes = 16<<10
const maxPublicTokenLength = 8192

var ErrPublicOCIUnavailable = errors.New("public OCI identity unavailable")

const indexAccept = ociIndexMediaType+", "+dockerIndexMediaType
const manifestAccept = ociManifestMediaType+", "+dockerManifestMediaType

// publicOCIClient implements only anonymous OCI Distribution reads. It
// refuses redirects, proxy configuration, credentials and alternate hosts.
// A test-only injected Doer permits hermetic auth/digest/timeout fixtures.
type publicOCIDoer interface { Do(*http.Request)(*http.Response,error) }
type publicOCIClient struct {client publicOCIDoer}

func defaultPublicOCIClient() publicOCIClient {
 return publicOCIClient{client:&http.Client{
  Timeout:publicOCITimeout,
  Transport:&http.Transport{Proxy:nil, TLSHandshakeTimeout:5*time.Second,
   ResponseHeaderTimeout:8*time.Second, MaxIdleConns:2, MaxIdleConnsPerHost:2},
  CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse},
 }}
}

func (h *HostReadOnlyAdapter) FetchOCI(ctx context.Context,repository,indexDigest string)([]byte,[]byte,error){
 if h==nil||h.clock==nil {return nil,nil,ErrPublicOCIUnavailable}
 return defaultPublicOCIClient().fetch(ctx,repository,indexDigest)
}

func (c publicOCIClient) fetch(ctx context.Context,repository,indexDigest string)([]byte,[]byte,error){
 if ctx==nil||ctx.Err()!=nil||c.client==nil||
  !executorregistration.ValidRepository(repository)||!validDigest(indexDigest){
  return nil,nil,ErrPublicOCIUnavailable
 }
 repo:=strings.TrimPrefix(repository,"docker.io/")
 index,typ,err:=c.manifest(ctx,repo,indexDigest,indexAccept,"")
 if err!=nil||!(typ==ociIndexMediaType||typ==dockerIndexMediaType){
  return nil,nil,ErrPublicOCIUnavailable
 }
 if digestFor(index)!=indexDigest{return nil,nil,ErrPublicOCIUnavailable}
 var parsed ociIndex
 if decodeOne(index,&parsed)!=nil||parsed.SchemaVersion!=2||
  parsed.MediaType!=typ||len(parsed.Manifests)==0||len(parsed.Manifests)>128{
  return nil,nil,ErrPublicOCIUnavailable
 }
 var selected *ociDescriptor
 for i:=range parsed.Manifests {
  d:=&parsed.Manifests[i]
  if d.Platform.OS=="linux"&&d.Platform.Architecture=="amd64"&&d.Platform.Variant==""{
   if selected!=nil{return nil,nil,ErrPublicOCIUnavailable}
   selected=d
  }
 }
 if selected==nil||!validDigest(selected.Digest)||selected.Size<=0||
  selected.Size>MaxOCIManifestBytes||
  (selected.MediaType!=ociManifestMediaType&&selected.MediaType!=dockerManifestMediaType){
  return nil,nil,ErrPublicOCIUnavailable
 }
 manifest,manifestType,err:=c.manifest(ctx,repo,selected.Digest,manifestAccept,"")
 if err!=nil||manifestType!=selected.MediaType||
  int64(len(manifest))!=selected.Size{return nil,nil,ErrPublicOCIUnavailable}
 if _,err=VerifyOCIPlatformProof(index,manifest,indexDigest);err!=nil{
  return nil,nil,ErrPublicOCIUnavailable
 }
 return index,manifest,nil
}

// GET is limited to two Docker Hub manifest endpoints. Every response is
// bounded and hashed, including child descriptor reads; 3xx is rejected.
func (c publicOCIClient) manifest(ctx context.Context,repo,reference,accept,unused string)([]byte,string,error){
 if !validDigest(reference)||!validRepositoryPath(repo){return nil,"",ErrPublicOCIUnavailable}
 endpoint:=dockerRegistryURL+"/v2/"+repo+"/manifests/"+reference
 request:=func(token string)(*http.Response,error){
  req,e:=http.NewRequestWithContext(ctx,http.MethodGet,endpoint,nil)
  if e!=nil{return nil,ErrPublicOCIUnavailable}
  req.Header.Set("Accept",accept)
  if token!=""{req.Header.Set("Authorization","Bearer "+token)}
  return c.client.Do(req)
 }
 response,err:=request("")
 if err!=nil{return nil,"",ErrPublicOCIUnavailable}
 if response.StatusCode==http.StatusUnauthorized {
  challenge:=response.Header.Get("WWW-Authenticate")
  _=response.Body.Close()
  token,e:=c.token(ctx,challenge,repo)
  if e!=nil{return nil,"",ErrPublicOCIUnavailable}
  response,err=request(token)
  if err!=nil{return nil,"",ErrPublicOCIUnavailable}
 }
 defer response.Body.Close()
 if response.StatusCode!=http.StatusOK{return nil,"",ErrPublicOCIUnavailable}
 mt,_,err:=mime.ParseMediaType(response.Header.Get("Content-Type"))
 if err!=nil||!(mt==ociIndexMediaType||mt==dockerIndexMediaType||
  mt==ociManifestMediaType||mt==dockerManifestMediaType){return nil,"",ErrPublicOCIUnavailable}
 capBytes:=MaxOCIManifestBytes
 if accept==indexAccept{capBytes=MaxOCIIndexBytes}
 data,err:=io.ReadAll(io.LimitReader(response.Body,int64(capBytes+1)))
 if err!=nil||len(data)==0||len(data)>capBytes||
  digestFor(data)!=reference{return nil,"",ErrPublicOCIUnavailable}
 header:=response.Header.Get("Docker-Content-Digest")
 if header!=""&&header!=reference{return nil,"",ErrPublicOCIUnavailable}
 return data,mt,nil
}

func validRepositoryPath(repo string)bool{
 return executorregistration.ValidRepository("docker.io/"+repo) && !strings.Contains(repo,"@")
}

func (c publicOCIClient) token(ctx context.Context,challenge,repo string)(string,error){
 // Only the exact Docker Hub bearer challenge, service and scope are
 // accepted. Never follow or reflect arbitrary realms, query strings or
 // credentials supplied in the challenge.
 if len(challenge)>2048||!strings.HasPrefix(challenge,"Bearer "){
  return "",ErrPublicOCIUnavailable
 }
 attrs,ok:=parseBearerChallenge(strings.TrimPrefix(challenge,"Bearer "))
 if !ok||attrs["realm"]!=dockerTokenURL||
  attrs["service"]!=dockerTokenService||
  attrs["scope"]!="repository:"+repo+":pull"{
  return "",ErrPublicOCIUnavailable
 }
 target,err:=url.Parse(dockerTokenURL)
 if err!=nil{return "",ErrPublicOCIUnavailable}
 q:=target.Query()
 q.Set("service",dockerTokenService)
 q.Set("scope","repository:"+repo+":pull")
 target.RawQuery=q.Encode()
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,target.String(),nil)
 if err!=nil{return "",ErrPublicOCIUnavailable}
 resp,err:=c.client.Do(req)
 if err!=nil{return "",ErrPublicOCIUnavailable}
 defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{return "",ErrPublicOCIUnavailable}
 data,err:=io.ReadAll(io.LimitReader(resp.Body,maxPublicTokenBytes+1))
 if err!=nil||len(data)==0||len(data)>maxPublicTokenBytes{return "",ErrPublicOCIUnavailable}
 var payload struct{Token string `json:"token"`; AccessToken string `json:"access_token"`}
 if json.Unmarshal(data,&payload)!=nil{return "",ErrPublicOCIUnavailable}
 token:=payload.Token
 if token==""{token=payload.AccessToken}
 if len(token)==0||len(token)>maxPublicTokenLength||
  strings.ContainsAny(token,"\x00\r\n \t"){return "",ErrPublicOCIUnavailable}
 return token,nil
}

// Reject duplicate challenge attributes or unquoted/unknown parameters,
// rather than trusting regexp-extracted fragments of attacker-controlled text.
func parseBearerChallenge(raw string)(map[string]string,bool){
 attrs:=map[string]string{}
 for raw!=""{
  raw=strings.TrimLeft(raw," ")
  idx:=strings.IndexByte(raw,'=')
  if idx<=0{return nil,false}
  key:=raw[:idx]
  if key!="realm"&&key!="service"&&key!="scope"{return nil,false}
  raw=raw[idx+1:]
  if len(raw)<2||raw[0]!='"'{return nil,false}
  raw=raw[1:]
  end:=strings.IndexByte(raw,'"')
  if end<0||end>1024{return nil,false}
  if _,exists:=attrs[key];exists{return nil,false}
  attrs[key]=raw[:end]
  raw=raw[end+1:]
  if raw!=""{
   if raw[0]!=','{return nil,false}
   raw=raw[1:]
   if raw==""{return nil,false}
  }
 }
 return attrs,len(attrs)==3
}
