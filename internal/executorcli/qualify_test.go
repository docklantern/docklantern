package executorcli

import (
 "bytes"
 "context"
 "errors"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/executorhost"
 "github.com/docklantern/docklantern/internal/executorpreflight"
 "github.com/docklantern/docklantern/internal/executorpreview"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

func TestQualificationCLIReadOnly(t *testing.T){
 calls:=0
 qualify:=func(_ context.Context,id string)(executorpreflight.Qualification,error){
  calls++
  if id!="proposal-demo"{t.Fatalf("unexpected proposal id: %q",id)}
  return executorpreflight.Qualification{ProposalID:id,RegistrationID:"registered-demo",
   Service:"web",CandidateDigest:"sha256:"+strings.Repeat("a",64),
   PlatformManifestDigest:"sha256:"+strings.Repeat("b",64),
   SourceSHA256:"sha256:"+strings.Repeat("c",64),ContainerID:strings.Repeat("d",64),
   ImageID:"sha256:"+strings.Repeat("e",64),ReadOnly:true},nil
 }
 var out,errOut bytes.Buffer
 rc:=RunWithQualification([]string{"qualify","proposal-demo"},&out,&errOut,"test",
  func(context.Context,executorhost.DiagnosticConfig)(executorhost.DiagnosticReport,error){
   t.Fatal("diagnose reached");return executorhost.DiagnosticReport{},nil
  },func(string,string)(executorregistration.Record,error){
   t.Fatal("register reached");return executorregistration.Record{},nil
  },func(string)(executorpreview.View,error){
   t.Fatal("preview reached");return executorpreview.View{},nil
  },qualify)
 if rc!=ExitOK||calls!=1||!strings.Contains(out.String(),"DOCKER WRITES DISABLED")||
  strings.Contains(out.String(),"approved to deploy"){
  t.Fatalf("read-only CLI result invalid: %d %q %q",rc,out.String(),errOut.String())
 }
}
func TestQualificationCLIFailsClosed(t *testing.T){
 cases:=[][]string{
  {"qualify"},{"qualify","../escape"},{"qualify","proposal-demo","--yes"},
  {"qualify","proposal-demo","--docker-host","ssh://other"},
 }
 for _,args:=range cases{
  calls:=0
  var out,errOut bytes.Buffer
  rc:=RunWithQualification(args,&out,&errOut,"test",nil,nil,nil,
   func(context.Context,string)(executorpreflight.Qualification,error){
    calls++;return executorpreflight.Qualification{ReadOnly:true},nil
   })
  if rc!=ExitUsage||calls!=0||out.Len()!=0{
   t.Fatalf("invalid CLI arguments accepted: %#v %d",args,rc)
  }
 }
 for _,tc:=range []struct{name string;result executorpreflight.Qualification;err error}{
  {"backend_denied",executorpreflight.Qualification{},errors.New("raw secret error")},
  {"not_readonly",executorpreflight.Qualification{ProposalID:"proposal-demo"},nil},
  {"mismatched_id",executorpreflight.Qualification{ProposalID:"other",ReadOnly:true},nil},
 }{
  t.Run(tc.name,func(t *testing.T){
   var out,errOut bytes.Buffer
   rc:=RunWithQualification([]string{"qualify","proposal-demo"},&out,&errOut,"test",nil,nil,nil,
    func(context.Context,string)(executorpreflight.Qualification,error){return tc.result,tc.err})
   if rc!=ExitUnavailable||out.Len()!=0||
    strings.Contains(errOut.String(),"raw secret"){
    t.Fatalf("denied preflight was accepted or leaked: %d %q",rc,errOut.String())
   }
  })
 }
}
