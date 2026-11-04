package executorpreflight

import (
 "context"
 "errors"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/executorproposal"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

type protectedQualificationFixture struct{
 proposal executorproposal.Record
 registration executorregistration.Record
 proposalReads,registrationReads int
 replaceProposal,replaceRegistration bool
}
func (s *protectedQualificationFixture)LoadProposal(id string,now time.Time)(executorproposal.Record,error){
 s.proposalReads++
 if id!=s.proposal.Proposal.ProposalID{return executorproposal.Record{},ErrLocalQualificationUnavailable}
 out:=s.proposal
 if s.replaceProposal&&s.proposalReads>1{out.SHA256="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
 return out,nil
}
func (s *protectedQualificationFixture)LoadRegistration(id string)(executorregistration.Record,error){
 s.registrationReads++
 if id!=s.registration.Registration.RegistrationID{return executorregistration.Record{},ErrLocalQualificationUnavailable}
 out:=s.registration
 if s.replaceRegistration&&s.registrationReads>1{out.SHA256="sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
 return out,nil
}
func TestLocalQualificationOrchestratesAndReopensTrustedInputs(t *testing.T){
 f:=makeQualificationFixture()
 source:=&protectedQualificationFixture{proposal:f.proposal,registration:f.reg}
 q,e:=QualifyFromSource(context.Background(),f.proposal.Proposal.ProposalID,source,
  f.reader,func()time.Time{return f.now})
 if e!=nil||!q.ReadOnly||q.ProposalSHA256!=f.proposal.SHA256||
  q.RegistrationSHA256!=f.reg.SHA256||source.proposalReads!=2||source.registrationReads!=2{
  t.Fatalf("read-only host qualification failed: %#v %v, source=%#v",q,e,source)
 }
}
func TestLocalQualificationRejectsProtectedInputReplacement(t *testing.T){
 for _,tc:=range []struct{name string;proposal,registration bool}{
  {"proposal_changed",true,false},{"registration_changed",false,true},
 }{
  t.Run(tc.name,func(t *testing.T){
   f:=makeQualificationFixture()
   source:=&protectedQualificationFixture{proposal:f.proposal,registration:f.reg,
    replaceProposal:tc.proposal,replaceRegistration:tc.registration}
   q,e:=QualifyFromSource(context.Background(),f.proposal.Proposal.ProposalID,source,
    f.reader,func()time.Time{return f.now})
   if !errors.Is(e,ErrLocalQualificationUnavailable)||q.ReadOnly{
    t.Fatalf("changed protected input qualified: %#v %v",q,e)
   }
  })
 }
}
func TestLocalQualificationRejectsInvalidProposalBeforeHostIO(t *testing.T){
 f:=makeQualificationFixture()
 s:=&protectedQualificationFixture{proposal:f.proposal,registration:f.reg}
 for _,id:=range []string{"../file","", "other-proposal"} {
  q,e:=QualifyFromSource(context.Background(),id,s,f.reader,func()time.Time{return f.now})
  if !errors.Is(e,ErrLocalQualificationUnavailable)||q.ReadOnly {
   t.Fatalf("invalid ID qualified: %#v %v",q,e)
  }
 }
}
