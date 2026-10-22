package registry

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync/atomic"
 "testing"
)
func digest(s string)string{b:=sha256.Sum256([]byte(s));return "sha256:"+hex.EncodeToString(b[:])}
func TestParse(t *testing.T){
 cases:=map[string]string{
  "nginx":"registry-1.docker.io/library/nginx:latest",
  "ubuntu:24.04":"registry-1.docker.io/library/ubuntu:24.04",
  "docker.io/team/api:v2":"registry-1.docker.io/team/api:v2",
  "ghcr.io/org/app:1.2.3":"ghcr.io/org/app:1.2.3",
  "registry.example:5000/team/app":"registry.example:5000/team/app:latest",
 }
 for input,want:=range cases {got,e:=Parse(input);if e!=nil||got.Key()!=want{t.Fatalf("%s: %+v %v",input,got,e)}}
 for _,s:=range []string{"","../bad","nginx@sha256:abc","GHCR.IO/org/web","ghcr.io/org/web:","nginx:bad tag","ghcr.io/org/../web"}{
  if _,e:=Parse(s);e==nil{t.Errorf("accepted invalid image %q",s)}
 }
}
func TestManifestAndPlatform(t *testing.T){
 child:=`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"digest":"sha256:fake"}}`
 index:=fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"digest":%q,"platform":{"os":"linux","architecture":"amd64"}}]}`,digest(child))
 var calls atomic.Int32
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  calls.Add(1)
  var body,typ string
  switch {
  case strings.HasSuffix(r.URL.Path,"/manifests/latest"):body=index;typ="application/vnd.oci.image.index.v1+json"
  case strings.HasSuffix(r.URL.Path,"/manifests/"+digest(child)):body=child;typ="application/vnd.oci.image.manifest.v1+json"
  default:http.NotFound(w,r);return
  }
  w.Header().Set("Content-Type",typ);w.Header().Set("Docker-Content-Digest",digest(body));_,_=w.Write([]byte(body))
 }))
 defer server.Close()
 c:=Client{HTTP:server.Client()}
 host:=strings.TrimPrefix(server.URL,"https://")
 got,e:=c.Check(context.Background(),host+"/team/web:latest","linux/amd64")
 if e!=nil{t.Fatal(e)}
 if got.IndexDigest!=digest(index)||got.PlatformDigest!=digest(child)||calls.Load()!=2{t.Fatalf("%+v calls=%d",got,calls.Load())}
 if _,e=c.Check(context.Background(),host+"/team/web:latest","linux/arm64");e==nil{t.Fatal("missing platform accepted")}
}
func TestBearerChallenge(t *testing.T){
 manifest:=`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json"}`
 var seenBasic,seenBearer atomic.Bool
 var host string
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path {
  case "/token":
   u,p,ok:=r.BasicAuth();if ok&&u=="bot"&&p=="secret"{seenBasic.Store(true)}
   if r.URL.Query().Get("scope")!="repository:team/private:pull"{t.Errorf("wrong scope")}
   _,_=w.Write([]byte(`{"token":"short-lived"}`))
  case "/v2/team/private/manifests/latest":
   if r.Header.Get("Authorization")!="Bearer short-lived"{
    w.Header().Set("WWW-Authenticate",fmt.Sprintf(`Bearer realm="https://%s/token",service="test",scope="repository:team/private:pull"`,host))
    w.WriteHeader(http.StatusUnauthorized);return
   }
   seenBearer.Store(true);w.Header().Set("Content-Type","application/vnd.oci.image.manifest.v1+json")
   w.Header().Set("Docker-Content-Digest",digest(manifest));_,_=w.Write([]byte(manifest))
  default:http.NotFound(w,r)
  }
 }))
 defer server.Close();host=strings.TrimPrefix(server.URL,"https://")
 c:=Client{HTTP:server.Client(),Credentials:Credentials{Username:"bot",Password:"secret"}}
 got,e:=c.Check(context.Background(),host+"/team/private:latest","linux/amd64")
 if e!=nil||got.PlatformDigest!=digest(manifest)||!seenBasic.Load()||!seenBearer.Load(){t.Fatalf("%+v %v auth=%v bearer=%v",got,e,seenBasic.Load(),seenBearer.Load())}
}
func TestDigestTampering(t *testing.T){
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Content-Type","application/vnd.oci.image.manifest.v1+json")
  w.Header().Set("Docker-Content-Digest","sha256:"+strings.Repeat("a",64))
  _,_=w.Write([]byte(`{"schemaVersion":2}`))
 }))
 defer server.Close()
 host:=strings.TrimPrefix(server.URL,"https://")
 _,e:=(Client{HTTP:server.Client()}).Check(context.Background(),host+"/repo:latest","linux/amd64")
 if e==nil||!strings.Contains(e.Error(),"digest"){t.Fatalf("tampered digest accepted: %v",e)}
}
func TestRejectedRealm(t *testing.T){
 c:=Client{}
 r:=Reference{Registry:"registry-1.docker.io",Repository:"library/nginx"}
 _,e:=c.bearer(context.Background(),`Bearer realm="http://example.org/auth",service="registry",scope="repository:library/nginx:pull"`,r)
 if e==nil{t.Fatal("insecure token realm accepted")}
}

func TestRejectCrossHostTokenRealm(t *testing.T){
 c:=Client{Credentials:Credentials{Username:"bot",Password:"do-not-forward"}}
 r:=Reference{Registry:"ghcr.io",Repository:"org/private"}
 _,e:=c.bearer(context.Background(),`Bearer realm="https://untrusted.example/token",service="ghcr.io",scope="repository:org/private:pull"`,r)
 if e==nil||!strings.Contains(e.Error(),"untrusted"){t.Fatalf("accepted unrelated token endpoint: %v",e)}
}
