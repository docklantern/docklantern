package executorapply

import (
 "context"
 "errors"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
)

type committingLedgerFixture struct{
 ledgerFixture
 finalized bool
 deny bool
}
func (l *committingLedgerFixture)FinalizeVerified(_ context.Context,id string,proof executorjournal.VerifiedEvidence,_ time.Time)error{
 l.events=append(l.events,"finalize:COMMIT")
 if l.deny||!l.intents["COMMIT"]||id==""||!proof.SiblingsUnchanged{
  return errors.New("unverified transaction denied")
 }
 l.finalized=true
 return nil
}
type witnessedEnvironmentFixture struct{
 environmentFixture
 deny bool
}
func(e *witnessedEnvironmentFixture)VerifiedEvidence(_ context.Context,op executorjournal.Operation)(executorjournal.VerifiedEvidence,error){
 if e.deny{return executorjournal.VerifiedEvidence{},errors.New("actual state drift")}
 if !strings.Contains(strings.Join(e.ledger.events,","),"complete:VERIFY"){
  return executorjournal.VerifiedEvidence{},errors.New("proof before verify")
 }
 return executorjournal.VerifiedEvidence{
  ProjectID:op.ProjectID,ServiceID:op.ServiceID,
  CandidateDigest:op.CandidateDigest,
  SourceSHA256:"sha256:"+strings.Repeat("c",64),
  BaselineContainerID:strings.Repeat("f",64),UpdatedContainerID:strings.Repeat("e",64),
  BaselineImageID:"sha256:"+strings.Repeat("a",64),
  UpdatedImageID:"sha256:"+strings.Repeat("b",64),
  HealthGrade:"healthy",SiblingsUnchanged:true,
 },nil
}
func TestJournaledCoordinatorRequiresVerifiedPromotionForSuccess(t *testing.T){
 op,q:=sampleCandidate()
 ledger:=&committingLedgerFixture{}
 environment:=&witnessedEnvironmentFixture{environmentFixture:environmentFixture{ledger:&ledger.ledgerFixture}}
 result,err:=RunCandidate(context.Background(),op,q,ledger,environment,func()time.Time{
  return op.CreatedAt.Add(50*time.Second)
 })
 if err!=nil||result.NeedsReconciliation||result.LastPhase!="COMMIT"||!ledger.finalized{
  t.Fatalf("complete independent verified promotion denied: %#v %v",result,err)
 }
 if !strings.Contains(strings.Join(ledger.events,","),"intent:COMMIT,finalize:COMMIT"){
  t.Fatalf("promotion not preceded by durable intent: %#v",ledger.events)
 }
}
func TestJournaledCoordinatorNeverReportsSuccessAfterVerifiedCommitFailure(t *testing.T){
 op,q:=sampleCandidate()
 for _,failure:=range []string{"no_witness","drift","commit_denied"}{
  t.Run(failure,func(t *testing.T){
   ledger:=&committingLedgerFixture{deny:failure=="commit_denied"}
   var environment Environment=&witnessedEnvironmentFixture{
    environmentFixture:environmentFixture{ledger:&ledger.ledgerFixture},
    deny:failure=="drift",
   }
   if failure=="no_witness"{environment=&environmentFixture{ledger:&ledger.ledgerFixture}}
   result,err:=RunCandidate(context.Background(),op,q,ledger,environment,time.Now)
   if !errors.Is(err,ErrApplyUnavailable)||!result.NeedsReconciliation||
    ledger.finalized{
    t.Fatalf("unverified commit returned success: %#v %v",result,err)
   }
  })
 }
}
