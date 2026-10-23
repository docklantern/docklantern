package management
import(
 "context"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/auth"
 appRuntime "github.com/docklantern/docklantern/internal/runtime"
)
func TestReadinessUnauthenticatedAndDistinctFromLiveness(t *testing.T){
 _,db,_,_:=setup(t)
 guard,err:=auth.New("admin",strings.Repeat("x",32));if err!=nil{t.Fatal(err)}
 h:=Handler(db,guard,nil,appRuntime.Checker{Reader:db,Required:[]appRuntime.Requirement{{Name:"registry_monitor",MaxAge:time.Minute}}})
 req:=func(path string)int{w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest("GET",path,nil));return w.Code}
 if req("/healthz")!=200||req("/readyz")!=503{t.Fatal("readiness incorrectly reported healthy before worker started")}
 if err=db.RecordWorkerRun(context.Background(),"registry_monitor",time.Now(),0,nil);err!=nil{t.Fatal(err)}
 if req("/readyz")!=200{t.Fatal("readiness did not recover")}
 if req("/api/v1/workers")!=401{t.Fatal("worker metadata should require auth")}
 if err=db.RecordWorkerRun(context.Background(),"registry_monitor",time.Now(),0,context.DeadlineExceeded);err!=nil{t.Fatal(err)}
 if req("/readyz")!=503||req("/healthz")!=200{t.Fatal("failed worker must not mark process liveness down")}
}
