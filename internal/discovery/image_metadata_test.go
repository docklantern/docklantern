package discovery
import(
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)
func TestProxyImageInspectByConfigID(t *testing.T){
 containerID:=strings.Repeat("a",64)
 configID:="sha256:"+strings.Repeat("b",64)
 digest:="sha256:"+strings.Repeat("c",64)
 var imageCalls int
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Method!="GET"{t.Fatal("unexpected Docker write")}
  switch r.URL.Path{
  case "/containers/json":
   _=json.NewEncoder(w).Encode([]any{map[string]any{"Id":containerID,"Labels":map[string]string{"com.docker.compose.project":"demo","com.docker.compose.service":"web"}}})
  case "/containers/"+containerID+"/json":
   _=json.NewEncoder(w).Encode(map[string]any{"Id":containerID,"Name":"/demo-web-1","Image":configID,"Config":map[string]any{"Image":"nginx:latest","Labels":map[string]string{"com.docker.compose.project":"demo","com.docker.compose.service":"web"}},"State":map[string]string{"Status":"running"}})
  case "/images/"+configID+"/json":
   imageCalls++
   _=json.NewEncoder(w).Encode(ImageMetadata{ID:configID,RepoDigests:[]string{"docker.io/library/nginx@"+digest},OS:"linux",Architecture:"amd64"})
  default:http.NotFound(w,r)
  }
 }))
 defer server.Close()
 p,_:=NewProxy(server.URL);items,err:=p.Scan(context.Background())
 if err!=nil||len(items)!=1||imageCalls!=1||items[0].RepoDigests[0]!="docker.io/library/nginx@"+digest||items[0].ImageOS!="linux"{t.Fatalf("%+v err=%v image calls=%d",items,err,imageCalls)}
}
func TestImageInspectMismatchNotEvidence(t *testing.T){
 id:="sha256:"+strings.Repeat("a",64)
 other:="sha256:"+strings.Repeat("b",64)
 if _,err:=verifiedImageMetadata(id,ImageMetadata{ID:other,OS:"linux",Architecture:"amd64",RepoDigests:[]string{"nginx@"+id}});err==nil{t.Fatal("accepted metadata from unrelated local image")}
 if _,err:=verifiedImageMetadata(id,ImageMetadata{ID:id,OS:"",Architecture:"amd64"});err==nil{t.Fatal("accepted absent platform")}
}
