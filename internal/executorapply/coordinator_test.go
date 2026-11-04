package executorapply

import (
 "context"
 "errors"
 "reflect"
 "strings"
 "testing"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executorpreflight"
)

type ledgerFixture struct{
 events []string
 failAt string
 intents map[string]bool
}
func (l *ledgerFixture)Begin(_ context.Context,_ executorjournal.Operation)error{
 l.events=append(l.events,"begin")
 if l.failAt=="begin"{return errors.New("storage failed")}
 l.intents=map[string]bool{}
 return nil
}
func (l *ledgerFixture)Intent(_ context.Context,_,phase string,_ time.Time)error{
 l.events=append(l.events,"intent:"+phase)
 if l.failAt=="intent:"+phase{return errors.New("storage failed")}
 l.intents[phase]=true
 return nil
}
func (l *ledgerFixture)Complete(_ context.Context,_,phase string,_ time.Time)error{
 l.events=append(l.events,"complete:"+phase)
 if l.failAt=="complete:"+phase{return errors.New("storage failed")}
 if !l.intents[phase]{return errors.New("missing intent")}
 delete(l.intents,phase)
 return nil
}
func (l *ledgerFixture)MarkUncertain(_ context.Context,_,phase,code string,_ time.Time)error{
 l.events=append(l.events,"uncertain:"+phase+":"+code)
 return nil
}

type environmentFixture struct{
 ledger *ledgerFixture
 failAt string
}
func (e *environmentFixture)act(phase string)error{
 e.ledger.events=append(e.ledger.events,"effect:"+phase)
 if !e.ledger.intents[phase]{return errors.New("effect without durable intent")}
 if e.failAt==phase{return errors.New("unsafe child stderr secret")}
 return nil
}
func (e *environmentFixture)ReviewApply(context.Context,executorjournal.Operation,executorpreflight.Qualification)error{return e.act("AUTHORIZE")}
func (e *environmentFixture)Requalify(context.Context,executorjournal.Operation,executorpreflight.Qualification)error{return e.act("PREFLIGHT")}
func (e *environmentFixture)PullImmutable(context.Context,executorjournal.Operation)error{return e.act("PULL")}
func (e *environmentFixture)StageOverride(context.Context,executorjournal.Operation)error{return e.act("STAGE")}
func (e *environmentFixture)ApplyTargetOnly(context.Context,executorjournal.Operation)error{return e.act("APPLY")}
func (e *environmentFixture)VerifyTargetAndSiblings(context.Context,executorjournal.Operation)error{return e.act("VERIFY")}

func sampleCandidate()(executorjournal.Operation,executorpreflight.Qualification){
 at:=time.Date(2026,11,4,4,0,0,0,time.UTC)
 digest:=func(c string)string{return "sha256:"+strings.Repeat(c,64)}
 op:=executorjournal.Operation{
  ID:"operation-test",Kind:"apply",ProjectID:"project-demo",
  RegistrationID:"registration-demo",ServiceID:"web",
  ProposalID:"proposal-demo",ProposalSHA256:digest("a"),
  CandidateDigest:digest("b"),CreatedAt:at,
 }
 q:=executorpreflight.Qualification{
  ProposalID:op.ProposalID,RegistrationID:op.RegistrationID,
  Service:op.ServiceID,CandidateDigest:op.CandidateDigest,
  SourceSHA256:digest("c"),PlatformManifestDigest:digest("d"),
  ImageID:digest("e"),ContainerID:strings.Repeat("f",64),
  ReadOnly:true,
 }
 return op,q
}
func TestJournaledCoordinatorNeverCommitsWithoutPromotion(t *testing.T){
 l:=&ledgerFixture{}
 e:=&environmentFixture{ledger:l}
 op,q:=sampleCandidate()
 result,err:=RunCandidate(context.Background(),op,q,l,e,func()time.Time{return op.CreatedAt.Add(time.Second)})
 if !errors.Is(err,ErrApplyUnavailable)||!result.NeedsReconciliation||result.LastPhase!="VERIFY"{
  t.Fatalf("premature success: %#v %v",result,err)
 }
 expected:=[]string{"begin"}
 for _,phase:=range []string{"PREPARE","AUTHORIZE","PREFLIGHT","PULL","STAGE","APPLY","VERIFY"}{
  expected=append(expected,"intent:"+phase)
  if phase!="PREPARE"{expected=append(expected,"effect:"+phase)}
  expected=append(expected,"complete:"+phase)
 }
 if !reflect.DeepEqual(l.events,expected){t.Fatalf("unexpected durable order: %#v",l.events)}
}
func TestJournaledCoordinatorStopsAtEachCrashBoundary(t *testing.T){
 phases:=[]string{"PREPARE","AUTHORIZE","PREFLIGHT","PULL","STAGE","APPLY","VERIFY"}
 for _,phase:=range phases{
  for _,failure:=range []string{"intent:"+phase,"complete:"+phase,"effect:"+phase}{
   if phase=="PREPARE"&&strings.HasPrefix(failure,"effect:"){continue}
   t.Run(failure,func(t *testing.T){
    op,q:=sampleCandidate()
    l:=&ledgerFixture{}
    e:=&environmentFixture{ledger:l}
    if strings.HasPrefix(failure,"effect:"){e.failAt=phase}else{l.failAt=failure}
    result,err:=RunCandidate(context.Background(),op,q,l,e,func()time.Time{return op.CreatedAt.Add(time.Second)})
    if !errors.Is(err,ErrApplyUnavailable)||!result.NeedsReconciliation||result.LastPhase!=phase{
     t.Fatalf("failure returned a success or continued: %#v %v",result,err)
    }
    seen:=false
    for _,ev:=range l.events{
     if ev=="intent:"+phase{seen=true}
     if strings.HasPrefix(ev,"effect:"+phase)&&!seen{t.Fatal("effect before durable intent")}
     if strings.HasPrefix(ev,"effect:")&&strings.TrimPrefix(ev,"effect:")!=phase&&
      strings.Index(strings.Join(phases,"/"),strings.TrimPrefix(ev,"effect:"))>strings.Index(strings.Join(phases,"/"),phase){
      t.Fatal("continued to later effect after failure")
     }
    }
   })
  }
 }
}
func TestJournaledCoordinatorRequiresQualificationBeforeBegin(t *testing.T){
 op,q:=sampleCandidate()
 cases:=[]func(*executorjournal.Operation,*executorpreflight.Qualification){
  func(o *executorjournal.Operation,q *executorpreflight.Qualification){q.ReadOnly=false},
  func(o *executorjournal.Operation,q *executorpreflight.Qualification){q.CandidateDigest="latest"},
  func(o *executorjournal.Operation,q *executorpreflight.Qualification){q.Service="sibling"},
  func(o *executorjournal.Operation,q *executorpreflight.Qualification){o.Kind="recover"},
  func(o *executorjournal.Operation,q *executorpreflight.Qualification){o.ProposalSHA256="short"},
  func(o *executorjournal.Operation,q *executorpreflight.Qualification){o.ProjectID="../escape"},
 }
 for i,f:=range cases{
  localOp,localQ:=op,q
  f(&localOp,&localQ)
  l:=&ledgerFixture{}
  e:=&environmentFixture{ledger:l}
  if _,err:=RunCandidate(context.Background(),localOp,localQ,l,e,time.Now);!errors.Is(err,ErrApplyUnavailable)||len(l.events)!=0{
   t.Fatalf("unqualified change %d created a journal: %v events=%v",i,err,l.events)
  }
 }
}
