// Package executorapply contains the first B6-05 journaled execution
// coordinator. Its mutation seam is injectable ONLY by trusted host code and
// disposable test fixtures. It is deliberately not wired into the production
// CLI, monitor, or a Docker subprocess yet.
package executorapply

import (
 "context"
 "errors"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executorpreflight"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrApplyUnavailable=errors.New("guarded executor apply unavailable")

// Ledger uses the existing durable SQLite journal in production and an
// interface in tests. Every phase's intent must be durably committed before
// calling an effect, even one believed to be read-only.
type Ledger interface{
 Begin(context.Context,executorjournal.Operation)error
 Intent(context.Context,string,string,time.Time)error
 Complete(context.Context,string,string,time.Time)error
 MarkUncertain(context.Context,string,string,string,time.Time)error
}

// Environment is purposefully narrow. Implementations may not accept
// arbitrary CLI commands or paths from a monitor proposal.
// Review must verify a fresh, exact host-local APPLY challenge.
// Inspect must verify target + siblings, not just the selected image.
type Environment interface{
 ReviewApply(context.Context,executorjournal.Operation,executorpreflight.Qualification)error
 Requalify(context.Context,executorjournal.Operation,executorpreflight.Qualification)error
 PullImmutable(context.Context,executorjournal.Operation)error
 StageOverride(context.Context,executorjournal.Operation)error
 ApplyTargetOnly(context.Context,executorjournal.Operation)error
 VerifyTargetAndSiblings(context.Context,executorjournal.Operation)error
}

type Result struct{
 OperationID string
 LastPhase string
 NeedsReconciliation bool
}

// RunCandidate executes a single, explicitly approved journaled prototype.
// It intentionally stops after VERIFY rather than claiming a successful
// COMMIT: the existing journal does not yet permit verified override
// promotion/succeeded state, and no operation may bypass that missing gate.
//
// The caller MUST acquire and retain a protected project lock, independently
// verify protected proposal/registration bytes, exact host runtime, and a
// source handoff through the complete subprocess. This coordinator alone
// does not supply those capabilities, thus production CLI remains disabled.
func RunCandidate(ctx context.Context,op executorjournal.Operation,q executorpreflight.Qualification,
 ledger Ledger,env Environment,clock func()time.Time)(Result,error){
 var zero Result
 if ctx==nil||ctx.Err()!=nil||ledger==nil||env==nil||clock==nil||
  op.Kind!="apply"||!executorregistration.ValidID(op.ID)||
  !executorregistration.ValidID(op.ProjectID)||
  !q.ReadOnly||q.ProposalID!=op.ProposalID||q.RegistrationID!=op.RegistrationID||
  q.Service!=op.ServiceID||q.CandidateDigest!=op.CandidateDigest||
  !sameDigest(q.SourceSHA256)||!sameDigest(q.PlatformManifestDigest)||
  !sameDigest(op.ProposalSHA256)||!sameDigest(op.CandidateDigest)||
  q.ContainerID==""||!sameDigest(q.ImageID){
  return zero,ErrApplyUnavailable
 }
 if ledger.Begin(ctx,op)!=nil{return zero,ErrApplyUnavailable}
 result:=Result{OperationID:op.ID,NeedsReconciliation:true}
 actions:=[]struct{
  phase string
  action func(context.Context,executorjournal.Operation)error
 }{
  {"PREPARE",func(ctx context.Context,op executorjournal.Operation)error{return nil}},
  {"AUTHORIZE",func(ctx context.Context,op executorjournal.Operation)error{return env.ReviewApply(ctx,op,q)}},
  {"PREFLIGHT",func(ctx context.Context,op executorjournal.Operation)error{return env.Requalify(ctx,op,q)}},
  {"PULL",env.PullImmutable},
  {"STAGE",env.StageOverride},
  {"APPLY",env.ApplyTargetOnly},
  {"VERIFY",env.VerifyTargetAndSiblings},
 }
 for _,step:=range actions{
  result.LastPhase=step.phase
  if ctx.Err()!=nil||ledger.Intent(ctx,op.ID,step.phase,clock())!=nil{
   // A failed intent may be an ambiguous storage outcome; do not invoke
   // any effect, and never attempt an unjournaled replay.
   return result,ErrApplyUnavailable
  }
  if ctx.Err()!=nil||step.action(ctx,op)!=nil{
   // Both failure and cancellation remain blocked until manual recovery.
   _=ledger.MarkUncertain(context.Background(),op.ID,step.phase,"state_ambiguous",clock())
   return result,ErrApplyUnavailable
  }
  if ledger.Complete(ctx,op.ID,step.phase,clock())!=nil{
   // Effect might already have happened. Never proceed to next phase.
   _=ledger.MarkUncertain(context.Background(),op.ID,step.phase,"storage_uncertain",clock())
   return result,ErrApplyUnavailable
  }
 }
 // No COMMIT or success status: absence of the final promotion contract
 // must leave the operation unresolved, including after successful Docker
 // replacement in a disposable fixture.
 result.LastPhase="VERIFY"
 return result,ErrApplyUnavailable
}
func sameDigest(s string)bool{
 if len(s)!=71||s[:7]!="sha256:"{return false}
 for _,c:=range s[7:]{
  if c<'0'||c>'9'&&c<'a'||c>'f'{return false}
 }
 return true
}
