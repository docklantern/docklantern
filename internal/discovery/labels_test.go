package discovery

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestMonitorLabelsWhitelist(t *testing.T) {
    input:=map[string]string{
        "com.docker.compose.project":"app",
        "com.docklantern.monitor":"true",
        "com.docklantern.interval":"5m",
        "com.example.secret":"private",
    }
    got:=monitorLabels(input)
    if len(got)!=2||got["com.docklantern.monitor"]!="true"||got["com.docklantern.interval"]!="5m" {
        t.Fatalf("missing monitor labels: %#v",got)
    }
    if _,ok:=got["com.example.secret"];ok{t.Fatal("forwarded unrelated label")}
    if x:=monitorLabels(nil);x!=nil{t.Fatalf("expected nil, got %#v",x)}
}

func TestProxyUsesInspectedMonitorLabels(t *testing.T) {
    id:=strings.Repeat("a",64)
    server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        if r.Method!=http.MethodGet {t.Fatal("unexpected Docker write")}
        switch r.URL.Path {
        case "/containers/json":
            _=json.NewEncoder(w).Encode([]map[string]any{{
                "Id":id,"Labels":map[string]string{
                    "com.docker.compose.project":"app",
                    "com.docker.compose.service":"web",
                    "com.docklantern.monitor":"false", // Untrusted summary must be ignored.
                },
            }})
        case "/containers/"+id+"/json":
            _=json.NewEncoder(w).Encode(map[string]any{
                "Id":id,"Name":"/app-web-1","Image":"sha256:short",
                "Config":map[string]any{
                    "Image":"nginx:latest","Labels":map[string]string{
                        "com.docker.compose.project":"app",
                        "com.docker.compose.service":"web",
                        "com.docklantern.monitor":"true",
                        "com.docklantern.interval":"15m",
                        "password":"never-copy",
                    },
                },
                "State":map[string]any{"Status":"running"},
            })
        default: http.NotFound(w,r)
        }
    }))
    defer server.Close()
    proxy,err:=NewProxy(server.URL);if err!=nil{t.Fatal(err)}
    found,err:=proxy.Scan(context.Background())
    if err!=nil||len(found)!=1{t.Fatalf("proxy scan failed: %+v %v",found,err)}
    labels:=found[0].MonitorLabels
    if labels["com.docklantern.monitor"]!="true"||labels["com.docklantern.interval"]!="15m"||len(labels)!=2 {
        t.Fatalf("wrong trusted labels: %#v",labels)
    }
}
