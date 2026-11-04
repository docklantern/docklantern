package executorpreflight

import (
 "context"
 "errors"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/executorregistration"
)

type stagedFixtureDelegate struct {
 *qualificationReader
 normalizedCalls int
 changedSource string
}
func (d *stagedFixtureDelegate) NormalizedComposeFromHandoff(
 ctx context.Context, reg executorregistration.Registration,
 handoff *SourceHandoff,service,candidate string)([]byte,error){
 d.normalizedCalls++
 if _,_,err:=handoff.FilesForReadOnlyCompose(reg);err!=nil{return nil,err}
 if d.changedSource!=""&&d.normalizedCalls==2{
  if err:=os.WriteFile(d.changedSource,[]byte("SAMPLE=unexpected-change\n"),0o600);err!=nil{return nil,err}
 }
 if candidate!=""{return d.after,nil}
 return d.before,nil
}

func TestStagedReadOnlyEvidenceQualificationRootFixture(t *testing.T){
 reg,_,_:=sourceFixture(t)
 f:=makeQualificationFixture()
 f.reg.Registration=reg
 f.proposal.Proposal.RegistrationID=reg.RegistrationID
 delegate:=&stagedFixtureDelegate{qualificationReader:f.reader}
 var stagedPath string
 wrapper:=newStagedReadOnlyEvidence(delegate,func(r executorregistration.Registration)(*SourceHandoff,error){
  h,err:=stageTrustedSourcesAt(r,"/root")
  if err==nil{stagedPath=h.directory}
  return h,err
 })
 q,err:=Qualify(context.Background(),f.proposal,f.reg,wrapper,func()time.Time{return f.now})
 if err!=nil||!q.ReadOnly||delegate.normalizedCalls!=4{
  t.Fatalf("staged OCI/Compose/actual qualification denied: %#v %v",q,err)
 }
 if wrapper.Close()!=nil{t.Fatal("stage cleanup failed")}
 if _,err:=os.Stat(stagedPath);!os.IsNotExist(err){t.Fatal("stage not removed")}
 if _,err:=wrapper.CaptureSources(context.Background(),reg);!errors.Is(err,ErrStagedEvidenceDenied){
  t.Fatal("closed stage reused",err)
 }
}
func TestStagedReadOnlyEvidenceRejectsSourceMutationRootFixture(t *testing.T){
 reg,_,env:=sourceFixture(t)
 f:=makeQualificationFixture()
 f.reg.Registration=reg
 f.proposal.Proposal.RegistrationID=reg.RegistrationID
 delegate:=&stagedFixtureDelegate{qualificationReader:f.reader,changedSource:env}
 wrapper:=newStagedReadOnlyEvidence(delegate,func(r executorregistration.Registration)(*SourceHandoff,error){
  return stageTrustedSourcesAt(r,"/root")
 })
 q,err:=Qualify(context.Background(),f.proposal,f.reg,wrapper,func()time.Time{return f.now})
 if !errors.Is(err,ErrQualificationDenied)||q.ReadOnly{
  t.Fatalf("trusted source changed during Compose was accepted: %#v %v",q,err)
 }
 if wrapper.Close()!=nil{t.Fatal("stage cleanup failed")}
}
func TestStagedReadOnlyEvidenceNoImplicitFallbackRootFixture(t *testing.T){
 reg,_,_:=sourceFixture(t)
 wrapper:=newStagedReadOnlyEvidence(&qualificationReader{},func(r executorregistration.Registration)(*SourceHandoff,error){
  return stageTrustedSourcesAt(r,"/root")
 })
 defer wrapper.Close()
 if _,err:=wrapper.CaptureSources(context.Background(),reg);err!=nil{t.Fatal(err)}
 if _,err:=wrapper.NormalizedCompose(context.Background(),reg,"web","");!errors.Is(err,ErrStagedEvidenceDenied){
  t.Fatal("mutable source fallback was accepted",err)
 }
}
func TestStagedReadOnlyEvidenceRefusesChangedRegistrationRootFixture(t *testing.T){
 reg,_,_:=sourceFixture(t)
 d:=&stagedFixtureDelegate{qualificationReader:&qualificationReader{}}
 wrapper:=newStagedReadOnlyEvidence(d,func(r executorregistration.Registration)(*SourceHandoff,error){
  return stageTrustedSourcesAt(r,"/root")
 })
 defer wrapper.Close()
 if _,err:=wrapper.CaptureSources(context.Background(),reg);err!=nil{t.Fatal(err)}
 reg.RegistrationID="different-registration"
 if _,err:=wrapper.CaptureSources(context.Background(),reg);!errors.Is(err,ErrStagedEvidenceDenied){
  t.Fatal("unrelated registration accepted",err)
 }
}
func TestStagedReadOnlyEvidenceRejectsUnavailableStage(t *testing.T){
 f:=makeQualificationFixture()
 w:=newStagedReadOnlyEvidence(f.reader,func(executorregistration.Registration)(*SourceHandoff,error){
  return nil,errors.New("raw host secret")
 })
 defer w.Close()
 if _,err:=w.CaptureSources(context.Background(),f.reg.Registration);!errors.Is(err,ErrStagedEvidenceDenied)||
  strings.Contains(err.Error(),"secret"){t.Fatal("stage error not sanitized",err)}
}
