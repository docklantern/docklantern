package management

import (
 "strings"
 "testing"
)

func TestBeta6DeploymentActivityIsReadOnlyAndUnknownWithoutMount(t *testing.T){
 h,_,_,_:=setup(t)
 response:=request(h,"GET","/deployments","",true)
 if response.Code!=200{t.Fatalf("activity page unavailable: %d: %s",response.Code,response.Body.String())}
 body:=response.Body.String()
 for _,text:=range []string{
  "Deployment Activity","No verified, read-only",
  "unknown","NO WEB EXECUTION","host-local operator confirmation",
  "No verified deployment receipts are available",
 }{
  if !strings.Contains(strings.ToLower(body),strings.ToLower(text)){
   t.Errorf("missing accurate receipt status %q",text)
  }
 }
 if strings.Contains(body,"Update now")||strings.Contains(body,"Deploy now")||
  strings.Contains(body,"Deployment successful"){
  t.Fatal("GUI offered an unsupported Docker operation or false success")
 }
 for _,path:=range []string{"/deployments","/"}{
  got:=request(h,"GET",path,"",true)
  if got.Code!=200||!strings.Contains(got.Body.String(),"href=\"/deployments\""){
   t.Fatalf("activity navigation absent from %s",path)
  }
 }
 if got:=request(h,"GET","/deployments","",false).Code;got!=401{
  t.Fatalf("unauthenticated user accessed deployment receipts: %d",got)
 }
 for _,method:=range []string{"POST","PUT","DELETE"}{
  got:=request(h,method,"/deployments","",true)
  if got.Code!=405{t.Fatalf("unexpected web execution route %s: %d",method,got.Code)}
 }
 if !strings.Contains(response.Header().Get("Content-Security-Policy"),"form-action 'none'"){
  t.Fatal("deployment Activity must refuse all forms")
 }
}
