package api
import(
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"
 "github.com/docklantern/docklantern/internal/store"
)
type fake struct{}
func(fake)Projects(context.Context)([]store.Registration,error){return []store.Registration{{Name:"demo"}},nil}
func(fake)Containers(context.Context)([]store.Container,error){return []store.Container{},nil}
func TestReadOnlyAPI(t *testing.T){
 h:=Handler(fake{})
 for _,path:=range []string{"/healthz","/api/v1/projects","/api/v1/containers"}{
  r:=httptest.NewRequest("GET",path,nil);w:=httptest.NewRecorder();h.ServeHTTP(w,r)
  if w.Code!=200||!json.Valid(w.Body.Bytes()){t.Fatalf("%s: %d %s",path,w.Code,w.Body.String())}
 }
 r:=httptest.NewRequest("POST","/api/v1/projects",nil);w:=httptest.NewRecorder();h.ServeHTTP(w,r);if w.Code!=405{t.Fatalf("mutation not blocked: %d",w.Code)}
}
