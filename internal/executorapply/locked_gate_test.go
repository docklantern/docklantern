package executorapply

import (
 "context"
 "errors"
 "regexp"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executorpreflight"
)

type gateLock struct{
 released int
 events *[]string
}
func (l *gateLock)Release()error{l.released++;*l.events=append(*l.events,"unlock");return nil}
type gateJournal struct{
 ledgerFixture
 closes int
}
func (j *gateJournal)Close()error{j.closes++;j.events=append(j.events,"close");return nil}
type gateTTY struct{
 interactive bool
 shown string
 reply func(string)string
 reads int
}
func (t *gateTTY)Interactive()bool{return t.interactive}
func (t *gateTTY)Print(s string)error{t.shown=s;return nil}
func (t *gateTTY)ReadLine(context.Context)(string,error){
 t.reads++
 if t.reply==nil{return "",errors.New("denied")}
 return t.reply(t.shown),nil
}
var challengeRE=regexp.MustCompile(`Type APPLY ([0-9a-f]{12})`)

func approvalResponse(shown string)string{
 match:=challengeRE.FindStringSubmatch(shown)
 if len(match)!=2{return ""}
 return "APPLY "+match[1]
}
func testLockedDeps(op executorjournal.Operation,q executorpreflight.Qualification)(LockedDependencies,*gateJournal,*gateLock,*gateTTY,*int){
 journal:=&gateJournal{}
 lock:=&gateLock{events:&journal.events}
 tty:=&gateTTY{interactive:true,reply:approvalResponse}
 qualifierCalls:=0
 now:=op.CreatedAt.Add(5*time.Second)
 q.CheckedAt=now
 d:=LockedDependencies{
  IsHostRoot:func()bool{return true},
  AcquireLock:func(id string)(ProjectLease,error){
   if id!=op.ProjectID{return nil,errors.New("invalid project")}
   journal.events=append(journal.events,"lock")
   return lock,nil
  },
  OpenJournal:func()(JournalSession,error){
   journal.events=append(journal.events,"open")
   return journal,nil
  },
  Qualify:func(ctx context.Context,id string)(executorpreflight.Qualification,error){
   if ctx.Err()!=nil||id!=op.ProposalID{return executorpreflight.Qualification{},errors.New("wrong proposal")}
   qualifierCalls++
   journal.events=append(journal.events,"qualify")
   return q,nil
  },
  Terminal:tty,
  Environment:&environmentFixture{ledger:&journal.ledgerFixture},
  Clock:func()time.Time{return now},
  Random:func(b []byte)(int,error){
   for i:=range b{b[i]=byte(i+1)}
   return len(b),nil
  },
 }
 return d,journal,lock,tty,&qualifierCalls
}

func TestLockedApplyGateOrderAndCleanup(t *testing.T){
 op,q:=sampleCandidate()
 d,journal,lock,tty,calls:=testLockedDeps(op,q)
 result,err:=RunLockedCandidate(context.Background(),op,d)
 if !errors.Is(err,ErrApplyUnavailable)||!result.NeedsReconciliation||
  result.LastPhase!="VERIFY"{t.Fatalf("unexpected terminal status: %#v %v",result,err)}
 if *calls!=3||tty.reads!=1||lock.released!=1||journal.closes!=1{
  t.Fatalf("missing host gate/cleanup: qualified=%d tty=%d lock=%d journal=%d",
   *calls,tty.reads,lock.released,journal.closes)
 }
 if !strings.HasPrefix(tty.shown,"DOCKLANTERN")||
  !strings.Contains(tty.shown,"may replace")||
  !strings.Contains(tty.shown,"Type APPLY "){
  t.Fatal("approval did not clearly describe a Docker mutation")
 }
 if len(journal.events)<9||journal.events[0]!="lock"||
  journal.events[1]!="open"||journal.events[2]!="qualify"{
  t.Fatalf("lock not held before evidence and journal work: %#v",journal.events)
 }
 for _,event:=range journal.events{
  if strings.Contains(event,"effect:AUTHORIZE"){t.Fatal("review-only effect was executed twice")}
 }
 // Closing the store must happen before releasing the host project lock.
 n:=len(journal.events)
 if journal.events[n-2]!="close"||journal.events[n-1]!="unlock"{
  t.Fatalf("lease released before journal closed: %#v",journal.events)
 }
}

func TestLockedApplyGateRejectsMissingOrWrongTTY(t *testing.T){
 op,q:=sampleCandidate()
 for _,tc:=range []struct{name string;setup func(*LockedDependencies,*gateTTY)}{
  {"nonroot",func(d *LockedDependencies,t *gateTTY){d.IsHostRoot=func()bool{return false}}},
  {"no_controlling_tty",func(d *LockedDependencies,t *gateTTY){t.interactive=false}},
  {"wrong_challenge",func(d *LockedDependencies,t *gateTTY){t.reply=func(string)string{return "APPLY 000000000000"}}},
  {"review_keyword_not_apply",func(d *LockedDependencies,t *gateTTY){t.reply=func(s string)string{return strings.Replace(approvalResponse(s),"APPLY","REVIEW",1)}}},
  {"no_response",func(d *LockedDependencies,t *gateTTY){t.reply=nil}},
  {"weak_nonce",func(d *LockedDependencies,t *gateTTY){d.Random=func([]byte)(int,error){return 0,nil}}},
  {"unqualified_host",func(d *LockedDependencies,t *gateTTY){d.Qualify=func(context.Context,string)(executorpreflight.Qualification,error){return executorpreflight.Qualification{},nil}}},
 }{
  t.Run(tc.name,func(t *testing.T){
   d,journal,lock,tty,_:=testLockedDeps(op,q)
   tc.setup(&d,tty)
   _,err:=RunLockedCandidate(context.Background(),op,d)
   if !errors.Is(err,ErrAuthorizationDenied){t.Fatalf("denial missing: %v",err)}
   if len(journal.ledgerFixture.intents)!=0||strings.Contains(strings.Join(journal.events,","),"effect:"){
    t.Fatalf("effect/journal begun before authorization: %#v",journal.events)
   }
   if tc.name!="nonroot"&&tc.name!="no_controlling_tty"&&lock.released!=1{
    t.Fatalf("held lock leaked on denial: %d",lock.released)
   }
  })
 }
}

func TestLockedApplyGateRechecksExternalDriftAfterTTY(t *testing.T){
 op,q:=sampleCandidate()
 d,journal,lock,tty,calls:=testLockedDeps(op,q)
 original:=d.Qualify
 d.Qualify=func(ctx context.Context,id string)(executorpreflight.Qualification,error){
  result,err:=original(ctx,id)
  if *calls==2{result.ContainerID=strings.Repeat("0",64)}
  return result,err
 }
 _,err:=RunLockedCandidate(context.Background(),op,d)
 if !errors.Is(err,ErrAuthorizationDenied)||tty.reads!=1||lock.released!=1||
  strings.Contains(strings.Join(journal.events,"|"),"begin"){
  t.Fatalf("external drift was accepted: %v %#v",err,journal.events)
 }
}

func TestLockedApplyGateRejectsFutureOrExpiredEvidence(t *testing.T){
 op,q:=sampleCandidate()
 for _,skew:=range []time.Duration{time.Minute,-time.Minute}{
  d,journal,_,_,_:=testLockedDeps(op,q)
  original:=d.Qualify
  d.Qualify=func(ctx context.Context,id string)(executorpreflight.Qualification,error){
   result,err:=original(ctx,id)
   result.CheckedAt=result.CheckedAt.Add(skew)
   return result,err
  }
  _,err:=RunLockedCandidate(context.Background(),op,d)
  if !errors.Is(err,ErrAuthorizationDenied)||len(journal.ledgerFixture.intents)!=0{
   t.Fatalf("stale/future qualification accepted: %v",err)
  }
 }
}
