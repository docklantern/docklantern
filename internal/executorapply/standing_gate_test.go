package executorapply

import (
 "context"
 "errors"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executorpreflight"
)

func standingFixture(t *testing.T) (executorjournal.Operation, StandingDependencies, *gateJournal,*gateLock,*int) {
 t.Helper()
 op,q:=sampleCandidate()
 manual,journal,lock,_,qualifications:=testLockedDeps(op,q)
 now:=op.CreatedAt.Add(5*time.Second)
 checks:=0
 deps:=StandingDependencies{
  IsHostRoot:func()bool{return true},
  AcquireLock:manual.AcquireLock,
  OpenJournal:manual.OpenJournal,
  Qualify:manual.Qualify,
  PrepareEnvironment:func(ctx context.Context,q executorpreflight.Qualification)(Environment,error){
   journal.events=append(journal.events,"prepare")
   return &environmentFixture{ledger:&journal.ledgerFixture},nil
  },
  CheckStanding:func(ctx context.Context)error{
   checks++
   journal.events=append(journal.events,"standing")
   return nil
  },
  NoChange:func(ctx context.Context)(bool,error){
   journal.events=append(journal.events,"unchanged")
   return false,nil
  },
  Clock:func()time.Time{return now},
 }
 return op,deps,journal,lock,qualifications
}

func TestStandingGateNoChangeIsNonMutating(t *testing.T) {
 op,deps,journal,lock,_:=standingFixture(t)
 deps.NoChange=func(context.Context)(bool,error){return true,nil}
 result,err:=RunLockedStandingCandidate(context.Background(),op,deps)
 if err!=nil||result.LastPhase!="UNCHANGED"||result.NeedsReconciliation||
  lock.released!=1||journal.closes!=1||len(journal.ledgerFixture.intents)!=0{
  t.Fatalf("unchanged path did not close without mutations: %#v %v %#v",result,err,journal.events)
 }
}

func TestStandingGateRequiresAuthorizationBeforeJournalEffects(t *testing.T){
 op,_,_,_,_:=standingFixture(t)
 for _,tc:=range []struct{name string;change func(*StandingDependencies)}{
  {"non_root",func(d *StandingDependencies){d.IsHostRoot=func()bool{return false}}},
  {"no_protected_policy",func(d *StandingDependencies){d.CheckStanding=nil}},
  {"policy_revoked",func(d *StandingDependencies){d.CheckStanding=func(context.Context)error{return errors.New("revoked")}}},
  {"drift",func(d *StandingDependencies){
   original:=d.Qualify
   seen:=0
   d.Qualify=func(ctx context.Context,id string)(executorpreflight.Qualification,error){
    seen++
    v,e:=original(ctx,id)
    if seen==2{v.ImageID="sha256:"+strings.Repeat("0",64)}
    return v,e
   }
  }},
  {"not_a_registration",func(d *StandingDependencies){d.OpenJournal=nil}},
 }{
  t.Run(tc.name,func(t *testing.T){
   _,d,journal,_,_:=standingFixture(t)
   tc.change(&d)
   _,err:=RunLockedStandingCandidate(context.Background(),op,d)
   if !errors.Is(err,ErrStandingAuthorization)||
    len(journal.ledgerFixture.intents)!=0{
    t.Fatalf("unauthorized standing update admitted: %v %#v",err,journal.events)
   }
  })
 }
}

func TestStandingGateUsesJournaledCoordinator(t *testing.T){
 op,deps,journal,lock,calls:=standingFixture(t)
 result,err:=RunLockedStandingCandidate(context.Background(),op,deps)
 // The test fixture deliberately supplies no independent verified COMMIT;
 // it must reach VERIFY, then refuse to claim a successful deployment.
 if !errors.Is(err,ErrApplyUnavailable)||!result.NeedsReconciliation||
  result.LastPhase!="VERIFY"||*calls!=3||lock.released!=1||journal.closes!=1{
  t.Fatalf("standing guard bypassed verified commit: %#v %v %#v",result,err,journal.events)
 }
 if !strings.Contains(strings.Join(journal.events,","),"standing")||
  strings.Contains(strings.Join(journal.events,","),"intent:COMMIT") {
  t.Fatalf("incorrect guarded execution path: %#v",journal.events)
 }
}
