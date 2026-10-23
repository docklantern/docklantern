package auth

import (
 "net/http"
 "net/http/httptest"
 "net/url"
 "strings"
 "testing"
)
func TestPublicOrigin(t *testing.T){
 g,_:=New("admin",strings.Repeat("s",32))
 for _,origin:=range []string{"http://example.com","https://example.com/path","https://user@example.com","https://example.com?q=1","https://example.com/#a"}{
  if err:=g.SetPublicOrigin(origin);err==nil{t.Fatalf("accepted %q",origin)}
 }
 if err:=g.SetPublicOrigin("https://docklantern.example.com");err!=nil{t.Fatal(err)}
 next:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(204)})
 h:=g.Wrap(next)
 cases:=[]struct{host,proto string;want int}{
  {"docklantern.example.com","https",204},
  {"attacker.example.com","https",403},
  {"docklantern.example.com","http",403},
  {"docklantern.example.com","",403},
 }
 for _,tc:=range cases{
  r:=httptest.NewRequest("GET","http://"+tc.host+"/settings",nil)
  r.Host=tc.host
  r.Header.Set("X-Forwarded-Proto",tc.proto)
  r.SetBasicAuth("admin",strings.Repeat("s",32))
  w:=httptest.NewRecorder();h.ServeHTTP(w,r)
  if w.Code!=tc.want{t.Errorf("host %s proto %s returned %d want %d",tc.host,tc.proto,w.Code,tc.want)}
 }
 values:=url.Values{"csrf_token":{g.Token()}}
 for _,origin:=range []string{"https://docklantern.example.com","http://docklantern.example.com","https://attacker.example.com"}{
  r:=httptest.NewRequest("POST","http://docklantern.example.com/settings/policies",strings.NewReader(values.Encode()))
  r.Header.Set("Content-Type","application/x-www-form-urlencoded")
  r.Header.Set("Origin",origin)
  want:=origin=="https://docklantern.example.com"
  if got:=g.CheckCSRF(r);got!=want{t.Errorf("Origin %s accepted=%v want %v",origin,got,want)}
 }
}
