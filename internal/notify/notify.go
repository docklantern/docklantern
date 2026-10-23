// Package notify delivers read-only monitoring events to one opt-in HTTPS webhook.
// A delivery is at-least-once; receivers should deduplicate by immutable event ID.
package notify

import (
 "bytes"
 "context"
 "crypto/hmac"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/netip"
 "net/url"
 "time"

 "github.com/docklantern/docklantern/internal/store"
)
type Sender struct{
 endpoint string
 secret []byte
 client *http.Client
}
func New(raw,secret string)(*Sender,error){
 if len(secret)<32{return nil,errors.New("webhook HMAC key must contain at least 32 characters")}
 u,err:=url.Parse(raw)
 if err!=nil||u.Scheme!="https"||u.Hostname()==""||u.User!=nil||u.Fragment!=""||u.RawQuery!=""{
  return nil,errors.New("webhook URL must be HTTPS without userinfo, query or fragment")
 }
 if len(raw)>2048{return nil,errors.New("webhook URL too long")}
 // No proxy env vars, redirects, or connections to private, loopback,
 // link-local or other non-global destinations (including DNS rebindings).
 dial:=func(ctx context.Context,network,address string)(net.Conn,error){
  host,port,err:=net.SplitHostPort(address);if err!=nil{return nil,errors.New("invalid webhook address")}
  ips,err:=net.DefaultResolver.LookupIPAddr(ctx,host)
  if err!=nil||len(ips)==0{return nil,errors.New("webhook DNS lookup failed")}
  for _,ip:=range ips{
   a,ok:=netip.AddrFromSlice(ip.IP)
   if !ok||!allowed(a){return nil,errors.New("webhook destination is not public")}
  }
  d:=net.Dialer{Timeout:5*time.Second}
  return d.DialContext(ctx,"tcp",net.JoinHostPort(ips[0].IP.String(),port))
 }
 client:=&http.Client{
  Timeout:10*time.Second,
  CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse},
  Transport:&http.Transport{Proxy:nil,DialContext:dial,TLSHandshakeTimeout:5*time.Second,ResponseHeaderTimeout:8*time.Second,DisableKeepAlives:true},
 }
 return &Sender{endpoint:u.String(),secret:[]byte(secret),client:client},nil
}
func allowed(a netip.Addr)bool{
 if a.Is4In6(){return allowed(a.Unmap())}
 if !a.IsGlobalUnicast()||a.IsPrivate()||a.IsLoopback()||a.IsLinkLocalUnicast()||a.IsLinkLocalMulticast(){return false}
 // Netip.IsGlobalUnicast alone includes non-public special allocations.
 for _,block:=range []netip.Prefix{
  netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
  netip.MustParsePrefix("192.0.0.0/24"), // IETF protocol assignments
  netip.MustParsePrefix("198.18.0.0/15"), // benchmarking/private labs
  netip.MustParsePrefix("192.0.2.0/24"), // documentation ranges
  netip.MustParsePrefix("198.51.100.0/24"),
  netip.MustParsePrefix("203.0.113.0/24"),
  netip.MustParsePrefix("2001:db8::/32"),
 }{
  if block.Contains(a){return false}
 }
 return true
}
func(s *Sender)Send(ctx context.Context,e store.Event)error{
 data,err:=json.Marshal(struct{
  Type string `json:"type"`
  Event store.Event `json:"event"`
 }{Type:"docklantern.monitoring.v1",Event:e})
 if err!=nil{return err}
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,s.endpoint,bytes.NewReader(data));if err!=nil{return err}
 mac:=hmac.New(sha256.New,s.secret);_,_=mac.Write(data)
 req.Header.Set("Content-Type","application/json")
 req.Header.Set("X-DockLantern-Signature","sha256="+hex.EncodeToString(mac.Sum(nil)))
 req.Header.Set("X-DockLantern-Event-ID",fmt.Sprintf("%d",e.ID))
 resp,err:=s.client.Do(req)
 if err!=nil{return errors.New("webhook connection failed")}
 defer resp.Body.Close()
 _,_=io.Copy(io.Discard,io.LimitReader(resp.Body,1024))
 if resp.StatusCode<200||resp.StatusCode>=300{return fmt.Errorf("webhook HTTP %d",resp.StatusCode)}
 return nil
}
type Outbox interface{
 DueEvents(context.Context,time.Time,int)([]store.Event,error)
 CompleteDelivery(context.Context,store.Event,time.Time,bool)error
}
type Worker struct {
 Store Outbox
 Sender *Sender
 Interval time.Duration
 Limit int
}
func(w Worker)RunOnce(ctx context.Context)(int,error){
 if w.Store==nil||w.Sender==nil||w.Limit<1||w.Limit>50{return 0,errors.New("invalid webhook worker configuration")}
 events,err:=w.Store.DueEvents(ctx,time.Now().UTC(),w.Limit)
 if err!=nil{return 0,err}
 for i,e:=range events{
  if err=ctx.Err();err!=nil{return i,err}
  sendCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
  sendErr:=w.Sender.Send(sendCtx,e)
  cancel()
  if err=w.Store.CompleteDelivery(ctx,e,time.Now().UTC(),sendErr==nil);err!=nil{return i,err}
 }
 return len(events),nil
}
func(w Worker)Run(ctx context.Context)error{
 if w.Interval<time.Second{return errors.New("webhook interval must be at least one second")}
 ticker:=time.NewTicker(w.Interval);defer ticker.Stop()
 for{
  if _,err:=w.RunOnce(ctx);err!=nil&&ctx.Err()==nil{return err}
  select{case <-ctx.Done():return nil;case <-ticker.C:}
 }
}
