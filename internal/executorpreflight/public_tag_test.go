package executorpreflight

import (
 "context"
 "errors"
 "net/http"
 "strings"
 "testing"
)

func TestPublicTagIsPinnedToVerifiedIndex(t *testing.T){
 index,manifest,digest:=ociFixture()
 child:=digestFor(manifest)
 calls:=0
 c:=publicOCIClient{client:ociFakeDoer(func(r *http.Request)(*http.Response,error){
  calls++
  if r.URL.Host!="registry-1.docker.io"||r.Method!=http.MethodGet{
   t.Fatalf("unexpected destination: %s",r.URL)
  }
  switch strings.TrimPrefix(r.URL.Path,"/v2/example/hello-go/manifests/"){
  case "latest":return ociResponse(200,ociIndexMediaType,string(index),digest,""),nil
  case digest:return ociResponse(200,ociIndexMediaType,string(index),digest,""),nil
  case child:return ociResponse(200,ociManifestMediaType,string(manifest),child,""),nil
  default:t.Fatalf("unexpected registry path: %s",r.URL.Path)
  }
  return nil,ErrPublicOCIUnavailable
 })}
 got,err:=c.resolveTag(context.Background(),"docker.io/example/hello-go","latest")
 if err!=nil||got!=digest||calls!=3{
  t.Fatalf("failed immutable tag verification: %s %v requests=%d",got,err,calls)
 }
}

func TestPublicTagRejectsUnverifiedOrUnsafeTag(t *testing.T){
 index,_,digest:=ociFixture()
 for _,tc:=range []struct{name,header string}{
  {"missing_header",""},{"forged_header","sha256:"+strings.Repeat("0",64)},
 }{
  t.Run(tc.name,func(t *testing.T){
   c:=publicOCIClient{client:ociFakeDoer(func(r *http.Request)(*http.Response,error){
    return ociResponse(200,ociIndexMediaType,string(index),tc.header,""),nil
   })}
   if _,e:=c.resolveTag(context.Background(),"docker.io/example/hello-go","latest");
    !errors.Is(e,ErrPublicOCIUnavailable){t.Fatalf("unverified registry tag accepted: %v",e)}
  })
 }
 count:=0
 c:=publicOCIClient{client:ociFakeDoer(func(r *http.Request)(*http.Response,error){
  count++
  return ociResponse(200,ociIndexMediaType,string(index),digest,""),nil
 })}
 for _,tag:=range []string{"../../etc","latest?x=1","",strings.Repeat("x",129)}{
  if _,e:=c.resolveTag(context.Background(),"docker.io/example/hello-go",tag);
   !errors.Is(e,ErrPublicOCIUnavailable){t.Fatal("invalid tag accepted",tag,e)}
 }
 if count!=0{t.Fatal("unsafe tag reached outbound transport")}
}
