package registry

import (
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
)
func TestRetryAfterParsing(t *testing.T){
 now:=time.Date(2026,10,22,0,0,0,0,time.UTC)
 tests:=[]struct{raw string;want time.Duration}{
  {"120",120*time.Second},
  {"999999",24*time.Hour},
  {"garbage",0},
  {"-5",0},
  {now.Add(90*time.Second).Format(http.TimeFormat),90*time.Second},
  {now.Add(-time.Minute).Format(http.TimeFormat),0},
 }
 for _,tc:=range tests{
  if got:=retryDelay(tc.raw,now);got!=tc.want{t.Errorf("%q = %v want %v",tc.raw,got,tc.want)}
 }
}
func TestRegistryRateLimitDoesNotLeakSecrets(t *testing.T){
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Retry-After","120")
  http.Error(w,"secret token should not escape",http.StatusTooManyRequests)
 }))
 defer server.Close()
 c:=Client{HTTP:server.Client(),Credentials:Credentials{Username:"robot",Password:"very-secret-token"}}
 _,err:=c.Check(context.Background(),strings.TrimPrefix(server.URL,"https://")+"/team/api:latest","linux/amd64")
 var retry *RetryableError
 if !errors.As(err,&retry)||retry.RetryAfter!=120*time.Second||retry.Status!=429{t.Fatalf("unexpected retry result: %v",err)}
 if strings.Contains(err.Error(),"secret"){t.Fatal("leaked private registry token")}
}
