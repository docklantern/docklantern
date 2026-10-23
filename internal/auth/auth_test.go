package auth

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)
func TestGuard(t *testing.T){
 if _,err:=New("admin","short");err==nil{t.Fatal("weak password accepted")}
 g,err:=New("admin",strings.Repeat("x",32));if err!=nil{t.Fatal(err)}
 h:=g.Wrap(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(204)}))
 for _,tc:=range []struct{user,pass string;want int}{{"","",401},{"admin","incorrect",401},{"admin",strings.Repeat("x",32),204}}{
  r:=httptest.NewRequest("GET","http://127.0.0.1:8787/api/v1/projects",nil)
  if tc.user!=""{r.SetBasicAuth(tc.user,tc.pass)}
  w:=httptest.NewRecorder();h.ServeHTTP(w,r)
  if w.Code!=tc.want{t.Fatalf("%q: %d want %d",tc.user,w.Code,tc.want)}
 }
 w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest("GET","/healthz",nil));if w.Code!=204{t.Fatal("health not accessible")}
}
func TestCSRF(t *testing.T){
 g,_:=New("admin",strings.Repeat("x",32))
 for _,tc:=range []struct{token,origin,fetch string;want bool}{
  {g.Token(),"http://127.0.0.1:8787","same-origin",true},
  {"missing","http://127.0.0.1:8787","same-origin",false},
  {g.Token(),"https://evil.example","cross-site",false},
  {g.Token(),"https://evil.example","",false},
 }{
  r:=httptest.NewRequest("POST","http://127.0.0.1:8787/settings/policies",strings.NewReader("csrf_token="+tc.token))
  r.Header.Set("Content-Type","application/x-www-form-urlencoded")
  if tc.origin!=""{r.Header.Set("Origin",tc.origin)}
  if tc.fetch!=""{r.Header.Set("Sec-Fetch-Site",tc.fetch)}
  if g.CheckCSRF(r)!=tc.want{t.Fatalf("csrf result for %s %s",tc.origin,tc.fetch)}
 }
}
