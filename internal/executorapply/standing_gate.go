package executorapply

import (
 "context"
 "errors"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executorpreflight"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrStandingAuthorization = errors.New("standing host update authorization unavailable")

// StandingDependencies is intentionally separate from LockedDependencies.
// Neither the host CLI's manual TTY approval nor monitor-originated proposals
// can select this gate. Only the host-native root entrypoint constructs it.
type StandingDependencies struct {
 IsHostRoot func() bool
 AcquireLock func(string) (ProjectLease, error)
 OpenJournal func() (JournalSession, error)
 Qualify func(context.Context,string) (executorpreflight.Qualification,error)
 PrepareEnvironment func(context.Context,executorpreflight.Qualification) (Environment,error)
 // CheckStanding reopens the protected registration, checks its original
 // exact-byte fingerprint, and re-resolves the approved remote tag.
 CheckStanding func(context.Context) error
 // NoChange must independently inspect persisted desired state and running
 // Docker identity under the same lock, never just compare registry watches.
 NoChange func(context.Context) (bool,error)
 Clock func() time.Time
}

// RunLockedStandingCandidate is the single automatic entry into the existing
// journaled phase coordinator. The only approval is a protected, explicit
// standing root registration; no terminal prompt is skipped in the manual
// RunLockedCandidate path. All effects retain the same project lock.
func RunLockedStandingCandidate(ctx context.Context, op executorjournal.Operation,
 d StandingDependencies) (result Result, resultErr error) {
 var zero Result
 if ctx==nil || ctx.Err()!=nil || d.IsHostRoot==nil || !d.IsHostRoot() ||
  d.AcquireLock==nil || d.OpenJournal==nil || d.Qualify==nil ||
  d.PrepareEnvironment==nil || d.CheckStanding==nil || d.NoChange==nil ||
  d.Clock==nil || op.Kind!="apply" ||
  !executorregistration.ValidID(op.ProjectID) ||
  !executorregistration.ValidID(op.ID) ||
  !executorregistration.ValidID(op.ProposalID) {
  return zero,ErrStandingAuthorization
 }
 lease,err:=d.AcquireLock(op.ProjectID)
 if err!=nil||lease==nil{return zero,ErrStandingAuthorization}
 defer lease.Release()
 journal,err:=d.OpenJournal()
 if err!=nil||journal==nil{return zero,ErrStandingAuthorization}
 defer func(){
  if journal.Close()!=nil {result.NeedsReconciliation=true;resultErr=ErrApplyUnavailable}
 }()
 if binder,ok:=journal.(interface{BindLease(ProjectLease)error});ok {
  if binder.BindLease(lease)!=nil{return zero,ErrStandingAuthorization}
 }
 if d.CheckStanding(ctx)!=nil{return zero,ErrStandingAuthorization}
 unchanged,err:=d.NoChange(ctx)
 if err!=nil{return zero,ErrStandingAuthorization}
 if unchanged{return Result{LastPhase:"UNCHANGED"},nil}

 initial,err:=d.Qualify(ctx,op.ProposalID)
 if err!=nil||!matchesOperation(initial,op,d.Clock()) {
  return zero,ErrStandingAuthorization
 }
 environment,err:=d.PrepareEnvironment(ctx,initial)
 if err!=nil||environment==nil{return zero,ErrStandingAuthorization}
 if closer,ok:=environment.(interface{Close()error});ok {
  defer func(){if closer.Close()!=nil{result.NeedsReconciliation=true;resultErr=ErrApplyUnavailable}}()
 }
 // A second exact standing-policy check replaces only the per-operation
 // TTY challenge. The immutable witness and remaining coordinator are shared.
 if d.CheckStanding(ctx)!=nil{return zero,ErrStandingAuthorization}
 fresh,err:=d.Qualify(ctx,op.ProposalID)
 if err!=nil||!matchesOperation(fresh,op,d.Clock())||!sameEvidence(initial,fresh){
  return zero,ErrStandingAuthorization
 }
 authorized:=&alreadyAuthorizedEnvironment{inner:environment,qualify:d.Qualify,
  clock:d.Clock,initial:fresh}
 return RunCandidate(ctx,op,fresh,journal,authorized,d.Clock)
}
