package notify

import(
 "context"
 "crypto/hmac"
 "crypto/sha256"
 "encoding/hex"
 "io"
 "net/http"
 "net/http/httptest"
 "net/netip"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/store"
)
func TestRejectUnsafeWebhookDestinations(t *testing.T){
 for _,raw:=range []string{"http://example.com/hook","https://user:pass@example.com","https://example.com/hook?token=hidden","https://example.com/#secret","ftp://example.com"}{
  if _,err:=New(raw,strings.Repeat("s",40));err==nil{t.Errorf("accepted %q",raw)}
 }
 for _,raw:=range []string{"127.0.0.1","10.0.0.2","192.168.1.1","169.254.169.254","100.64.0.1","198.18.0.1","192.0.0.9","::1","fd00::1","2001:db8::1"}{
  ip:=netip.MustParseAddr(raw);if allowed(ip){t.Errorf("allowed private IP %s",ip)}
 }
 for _,raw:=range []string{"8.8.8.8","2606:4700:4700::1111"}{
  if !allowed(netip.MustParseAddr(raw)){t.Errorf("blocked public IP %s",raw)}
 }
}
func TestSignedWebhookDelivery(t *testing.T){
 secret:=strings.Repeat("k",40)
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  data,err:=io.ReadAll(io.LimitReader(r.Body,4096));if err!=nil{t.Error(err)}
  mac:=hmac.New(sha256.New,[]byte(secret));_,_=mac.Write(data)
  want:="sha256="+hex.EncodeToString(mac.Sum(nil))
  if r.Header.Get("X-DockLantern-Signature")!=want||r.Header.Get("X-DockLantern-Event-ID")!="42"||r.Method!="POST"{t.Error("webhook signature or identity mismatch")}
  w.WriteHeader(204)
 }))
 defer server.Close()
 sender,err:=New(server.URL+"/events",secret);if err!=nil{t.Fatal(err)}
 // Local fixture only: production client rejects loopback even when DNS rebinds.
 sender.client=server.Client()
 if err=sender.Send(context.Background(),store.Event{ID:42,Kind:"remote_digest_changed",Image:"nginx:latest"});err!=nil{t.Fatal(err)}
}
