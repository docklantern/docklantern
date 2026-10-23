package notify
import(
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
 "github.com/docklantern/docklantern/internal/store"
)
type fakeOutbox struct{events []store.Event;delivered int;failed int}
func(f *fakeOutbox)DueEvents(context.Context,time.Time,int)([]store.Event,error){return f.events,nil}
func(f *fakeOutbox)CompleteDelivery(_ context.Context,_ store.Event,_ time.Time,ok bool)error{if ok{f.delivered++}else{f.failed++};return nil}
func TestWorkerRetriesDeliveryWithoutMutatingMonitoring(t *testing.T){
 count:=0
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  count++;if count==1{http.Error(w,"temporary failure",503)}else{w.WriteHeader(204)}
 }))
 defer server.Close()
 sender,err:=New(server.URL,strings.Repeat("s",40));if err!=nil{t.Fatal(err)}
 sender.client=server.Client() // Fixture permits local TLS; production dialer rejects it.
 f:=&fakeOutbox{events:[]store.Event{{ID:7,Kind:"remote_digest_changed",Image:"ghcr.io/acme/api:latest"}}}
 worker:=Worker{Store:f,Sender:sender,Limit:5,Interval:time.Second}
 if n,err:=worker.RunOnce(context.Background());err!=nil||n!=1||f.failed!=1{t.Fatalf("first: %d %v %+v",n,err,f)}
 if n,err:=worker.RunOnce(context.Background());err!=nil||n!=1||f.delivered!=1{t.Fatalf("second: %d %v %+v",n,err,f)}
}
