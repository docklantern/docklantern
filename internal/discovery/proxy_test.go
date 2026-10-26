package discovery
import(
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync/atomic"
 "testing"
)
func TestProxyScan(t *testing.T){
 id:=strings.Repeat("a",64)
 var calls atomic.Int32
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Method!="GET"{t.Fatalf("write attempted: %s",r.Method)}
  calls.Add(1)
  switch r.URL.Path{
  case "/containers/json":
   if r.URL.Query().Get("all")!="1"{t.Error("missing all=1")}
   _=json.NewEncoder(w).Encode([]map[string]any{
    {"Id":id,"Labels":map[string]string{"com.docker.compose.project":"sample","com.docker.compose.service":"web","com.docklantern.monitor":"true","com.docklantern.interval":"10m","other.secret":"hidden"}},
    {"Id":strings.Repeat("b",64),"Labels":map[string]string{"unrelated":"yes"}},
   })
  case "/containers/"+id+"/json":
   _=json.NewEncoder(w).Encode(map[string]any{"Id":id,"Name":"/sample-web-1","Image":"sha256:abc","Config":map[string]any{"Image":"nginx:1.27","Labels":map[string]string{"com.docker.compose.project":"sample","com.docker.compose.service":"web"}},"State":map[string]any{"Status":"running","Health":map[string]string{"Status":"healthy"}}})
  default:http.NotFound(w,r)
  }
 }))
 defer server.Close()
 p,err:=NewProxy(server.URL);if err!=nil{t.Fatal(err)}
 found,err:=p.Scan(context.Background())
 if err!=nil||len(found)!=1||found[0].Health!="healthy"||found[0].Image!="nginx:1.27"||calls.Load()!=2{t.Fatalf("%+v %v calls=%d",found,err,calls.Load())}
 if len(found[0].MonitorLabels)!=0{t.Fatal("summary labels were trusted without inspect detail")}
}
func TestProxyFailClosed(t *testing.T){
 for _,url:=range []string{"","https://remote.invalid","http://user:pass@proxy:2375","http://proxy:2375/path","http://proxy:2375?all=1","http://0.0.0.0:2375"}{
  if _,err:=NewProxy(url);err==nil{t.Errorf("accepted %q",url)}
 }
 id:=strings.Repeat("a",64)
 for _,tc:=range []struct{name string;response string; code int}{
  {"invalid id",`[{"Id":"../../bad","Labels":{"com.docker.compose.project":"a","com.docker.compose.service":"b"}}]`,200},
  {"too many",func()string{var a []map[string]any;for i:=0;i<501;i++{a=append(a,map[string]any{"Id":fmt.Sprintf("%064x",i),"Labels":map[string]string{"com.docker.compose.project":"a","com.docker.compose.service":"b"}})};b,_:=json.Marshal(a);return string(b)}(),200},
  {"forbidden","forbidden",403},
  {"bad json","{oops",200},
 }{
  t.Run(tc.name,func(t *testing.T){
   server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(tc.code);_,_=w.Write([]byte(tc.response))}))
   defer server.Close()
   p,_:=NewProxy(server.URL)
   if _,err:=p.Scan(context.Background());err==nil{t.Fatal("scan should reject unsafe input")}
   _=id
  })
 }
}
func TestProxyInspectIdentityMismatch(t *testing.T){
 id:=strings.Repeat("a",64)
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path=="/containers/json"{_ =json.NewEncoder(w).Encode([]map[string]any{{"Id":id,"Labels":map[string]string{"com.docker.compose.project":"p","com.docker.compose.service":"s"}}});return}
  _=json.NewEncoder(w).Encode(map[string]any{"Id":strings.Repeat("b",64)})
 }))
 defer server.Close()
 p,_:=NewProxy(server.URL)
 if _,err:=p.Scan(context.Background());err==nil{t.Fatal("identity mismatch accepted")}
}
