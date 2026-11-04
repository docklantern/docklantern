package executorpreflight

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strings"
 "testing"
)

type ociFakeDoer func(*http.Request)(*http.Response,error)
func (f ociFakeDoer)Do(req *http.Request)(*http.Response,error){return f(req)}
func ociResponse(code int,contentType,body,digest,challenge string)*http.Response{
 head:=make(http.Header)
 if contentType!=""{head.Set("Content-Type",contentType)}
 if digest!=""{head.Set("Docker-Content-Digest",digest)}
 if challenge!=""{head.Set("WWW-Authenticate",challenge)}
 return &http.Response{StatusCode:code,Header:head,Body:io.NopCloser(strings.NewReader(body))}
}
func publicFixture(t *testing.T)(publicOCIClient,string,string,string){
 t.Helper()
 index,manifest,digest:=ociFixture()
 repo:="example/hello-go"
 child:=digestFor(manifest)
 count:=0
 challenge:=`Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:example/hello-go:pull"`
 client:=publicOCIClient{client:ociFakeDoer(func(req *http.Request)(*http.Response,error){
  count++
  if req.Method!=http.MethodGet||req.URL.Scheme!="https"||
   req.URL.User!=nil||req.Header.Get("Cookie")!=""||
   req.Header.Get("Proxy-Authorization")!=""{t.Fatalf("untrusted outbound request: %#v",req)}
  switch req.URL.Host {
  case "auth.docker.io":
   if req.URL.Path!="/token"||req.URL.Query().Get("scope")!="repository:"+repo+":pull"||
    req.URL.Query().Get("service")!=dockerTokenService||
    req.Header.Get("Authorization")!=""{t.Fatal("token request redirected or credentials leaked")}
   return ociResponse(200,"application/json",`{"token":"opaque-test-token"}`,"",""),nil
  case "registry-1.docker.io":
   if req.URL.Path!="/v2/"+repo+"/manifests/"+digest&&
    req.URL.Path!="/v2/"+repo+"/manifests/"+child{
    t.Fatalf("unapproved manifest path %q",req.URL.Path)
   }
   if req.Header.Get("Authorization")==""{
    return ociResponse(401,"","", "",challenge),nil
   }
   if req.Header.Get("Authorization")!="Bearer opaque-test-token"{
    t.Fatal("unexpected auth token")
   }
   if req.URL.Path=="/v2/"+repo+"/manifests/"+digest{
    return ociResponse(200,ociIndexMediaType,string(index),digest,""),nil
   }
   return ociResponse(200,ociManifestMediaType,string(manifest),child,""),nil
  default:t.Fatalf("untrusted outbound host %q",req.URL.Host)
  }
  return nil,ErrPublicOCIUnavailable
 })}
 return client,repo,digest,child
}
func TestPublicOCIDockerHubAnonymousBearerAndDigest(t *testing.T){
 client,repo,hash,child:=publicFixture(t)
 index,manifest,err:=client.fetch(context.Background(),"docker.io/"+repo,hash)
 if err!=nil{t.Fatal(err)}
 proof,err:=VerifyOCIPlatformProof(index,manifest,hash)
 if err!=nil||proof.PlatformManifestDigest!=child{t.Fatalf("platform proof failed: %#v %v",proof,err)}
}

func TestPublicOCIRejectsMalformedAndUntrustedRequests(t *testing.T){
 client,_,hash,_:=publicFixture(t)
 for _,name:=range []string{
  "evil.invalid/example","docker.io/library/one/extra",
  "docker.io/example/../hello","docker.io/example/hello@sha256:bad",
  "docker.io/example/Hello","http://docker.io/example/hello",
 }{
  if _,_,e:=client.fetch(context.Background(),name,hash);!errors.Is(e,ErrPublicOCIUnavailable){
   t.Fatalf("untrusted repository accepted: %q %v",name,e)
  }
 }
 if _,_,e:=client.fetch(context.Background(),"docker.io/example/hello-go","sha256:bad");!errors.Is(e,ErrPublicOCIUnavailable){
  t.Fatal("invalid index digest accepted")
 }
}

func TestPublicOCIRejectsUnexpectedAuthChallenges(t *testing.T){
 _,_,hash,_:=publicFixture(t)
 cases:=[]string{
  `Bearer realm="https://attacker.invalid/token",service="registry.docker.io",scope="repository:example/hello-go:pull"`,
  `Bearer realm="http://auth.docker.io/token",service="registry.docker.io",scope="repository:example/hello-go:pull"`,
  `Bearer realm="https://auth.docker.io/token",service="wrong",scope="repository:example/hello-go:pull"`,
  `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:another/repo:pull"`,
  `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:example/hello-go:pull",realm="https://attacker.invalid"`,
  `Basic realm="https://auth.docker.io/token"`,
  `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope=repository:example/hello-go:pull`,
 }
 for _,challenge:=range cases{t.Run(challenge,func(t *testing.T){
  calls:=0
  c:=publicOCIClient{client:ociFakeDoer(func(req *http.Request)(*http.Response,error){
   calls++
   if calls>1{t.Fatal("followed unsafe challenge")}
   return ociResponse(401,"","","",challenge),nil
  })}
  if _,_,err:=c.fetch(context.Background(),"docker.io/example/hello-go",hash);
   !errors.Is(err,ErrPublicOCIUnavailable){t.Fatalf("unsafe realm accepted %v",err)}
  if calls!=1{t.Fatalf("unexpected outbound call count: %d",calls)}
 })}
}
func TestPublicOCIRejectsWrongIndexOrChildEvidence(t *testing.T){
 idx,manifest,hash:=ociFixture()
 child:=digestFor(manifest)
 tests:=[]struct{name string;respond func(*http.Request)*http.Response}{
  {"redirect",func(r *http.Request)*http.Response{return ociResponse(302,"","","","")}},
  {"wrong_header_digest",func(r *http.Request)*http.Response{return ociResponse(200,ociIndexMediaType,string(idx),"sha256:"+strings.Repeat("f",64),"")}},
  {"corrupt_index",func(r *http.Request)*http.Response{return ociResponse(200,ociIndexMediaType,string(append(bytes.Clone(idx),' ')),hash,"")}},
  {"oversized_index",func(r *http.Request)*http.Response{return ociResponse(200,ociIndexMediaType,strings.Repeat("x",MaxOCIIndexBytes+1),hash,"")}},
  {"child_corruption",func(r *http.Request)*http.Response{
   if strings.HasSuffix(r.URL.Path,hash){return ociResponse(200,ociIndexMediaType,string(idx),hash,"")}
   return ociResponse(200,ociManifestMediaType,string(append(bytes.Clone(manifest),' ')),child,"")
  }},
  {"wrong_child_media_type",func(r *http.Request)*http.Response{
   if strings.HasSuffix(r.URL.Path,hash){return ociResponse(200,ociIndexMediaType,string(idx),hash,"")}
   return ociResponse(200,dockerManifestMediaType,string(manifest),child,"")
  }},
  {"duplicate_amd64_descriptor",func(r *http.Request)*http.Response{
   var x map[string]any
   _=json.Unmarshal(idx,&x)
   list:=x["manifests"].([]any)
   x["manifests"]=append(list,list[0])
   changed,_:=json.Marshal(x)
   return ociResponse(200,ociIndexMediaType,string(changed),digestFor(changed),"")
  }},
 }
 for _,tc:=range tests{t.Run(tc.name,func(t *testing.T){
  c:=publicOCIClient{client:ociFakeDoer(func(req *http.Request)(*http.Response,error){
   return tc.respond(req),nil
  })}
  expected:=hash
  if tc.name=="duplicate_amd64_descriptor"{
   var x map[string]any
   _=json.Unmarshal(idx,&x)
   list:=x["manifests"].([]any)
   x["manifests"]=append(list,list[0])
   bytes,_:=json.Marshal(x)
   expected=digestFor(bytes)
  }
  if _,_,err:=c.fetch(context.Background(),"docker.io/example/hello-go",expected);
   !errors.Is(err,ErrPublicOCIUnavailable){t.Fatalf("unsafe evidence accepted: %v",err)}
 })}
}

func TestPublicOCIRejectsMissingOrMalformedToken(t *testing.T){
 _,_,hash,_:=publicFixture(t)
 challenge:=`Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:example/hello-go:pull"`
 for _,tokenBody:=range []string{`{}`,`{"token":""}`,`{"token":"bad token"}`,`{"token":"a\nb"}`,strings.Repeat("x",maxPublicTokenBytes+1)}{
  calls:=0
  c:=publicOCIClient{client:ociFakeDoer(func(req *http.Request)(*http.Response,error){
   calls++
   if req.URL.Host=="auth.docker.io" {
    return ociResponse(200,"application/json",tokenBody,"",""),nil
   }
   if calls>2 {t.Fatal("unexpected manifest retry")}
   return ociResponse(401,"","","",challenge),nil
  })}
  if _,_,err:=c.fetch(context.Background(),"docker.io/example/hello-go",hash);
   !errors.Is(err,ErrPublicOCIUnavailable){t.Fatalf("invalid token accepted: %v",err)}
 }
}
