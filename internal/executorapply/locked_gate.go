package executorapply

import (
 "context"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executorpreflight"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrAuthorizationDenied=errors.New("host-local APPLY authorization denied")

const MaxApplyApproval=2*time.Minute
const MaxApplyEvidenceAge=10*time.Second

// Terminal is an actual controlling terminal in the eventual root host
// adapter, never web stdin, an HTTP request or an untrusted proposal field.
type Terminal interface{
 Interactive()bool
 Print(string)error
 ReadLine(context.Context)(string,error)
}

type ProjectLease interface{Release()error}
type JournalSession interface{
 Ledger
 Close()error
}

// LockedDependencies is a seam used for isolated tests. The future production
// adapter must provide AcquireProjectLock, OpenHost, QualifyLocal's protected
// proposal/registration readers and an actual host-root controlling TTY.
// No automatic host adapter is provided by this PR.
type LockedDependencies struct{
 IsHostRoot func()bool
 AcquireLock func(string)(ProjectLease,error)
 OpenJournal func()(JournalSession,error)
 Qualify func(context.Context,string)(executorpreflight.Qualification,error)
 Terminal Terminal
 Environment Environment
 Clock func()time.Time
 Random func([]byte)(int,error)
}

// RunLockedCandidate holds the project lock across approval, repeated fresh
// qualification, durable journal writes and every injected effect. It does
// not expose a production CLI path, and cannot report success/commit.
func RunLockedCandidate(ctx context.Context,op executorjournal.Operation,d LockedDependencies)(Result,error){
 var zero Result
 if ctx==nil||ctx.Err()!=nil||d.IsHostRoot==nil||!d.IsHostRoot()||
  d.AcquireLock==nil||d.OpenJournal==nil||d.Qualify==nil||
  d.Terminal==nil||!d.Terminal.Interactive()||
  d.Environment==nil||d.Clock==nil||d.Random==nil||
  op.Kind!="apply"||!executorregistration.ValidID(op.ProjectID)||
  !executorregistration.ValidID(op.ID)||!executorregistration.ValidID(op.ProposalID){
  return zero,ErrAuthorizationDenied
 }
 lock,err:=d.AcquireLock(op.ProjectID)
 if err!=nil||lock==nil{return zero,ErrAuthorizationDenied}
 // Close errors are not interpreted as a successful release or deployment.
 defer lock.Release()
 journal,err:=d.OpenJournal()
 if err!=nil||journal==nil{return zero,ErrAuthorizationDenied}
 defer journal.Close()
 initial,err:=d.Qualify(ctx,op.ProposalID)
 if err!=nil||!matchesOperation(initial,op,d.Clock()){
  return zero,ErrAuthorizationDenied
 }
 if approveApply(ctx,op,initial,d.Terminal,d.Clock,d.Random)!=nil{
  return zero,ErrAuthorizationDenied
 }
 // After human input, refresh all evidence again under the same lock.
 fresh,err:=d.Qualify(ctx,op.ProposalID)
 if err!=nil||!matchesOperation(fresh,op,d.Clock())||!sameEvidence(initial,fresh){
  return zero,ErrAuthorizationDenied
 }
 environment:=&alreadyAuthorizedEnvironment{inner:d.Environment,qualify:d.Qualify,clock:d.Clock,initial:fresh}
 return RunCandidate(ctx,op,fresh,journal,environment,d.Clock)
}

type alreadyAuthorizedEnvironment struct{
 inner Environment
 qualify func(context.Context,string)(executorpreflight.Qualification,error)
 clock func()time.Time
 initial executorpreflight.Qualification
}
func (a *alreadyAuthorizedEnvironment)ReviewApply(ctx context.Context,op executorjournal.Operation,q executorpreflight.Qualification)error{
 if ctx.Err()!=nil||!sameEvidence(q,a.initial)||!matchesOperation(q,op,a.clock()){
  return ErrAuthorizationDenied
 }
 // The only APPLY approval is the fresh host-local challenge above.
 return nil
}
func (a *alreadyAuthorizedEnvironment)Requalify(ctx context.Context,op executorjournal.Operation,q executorpreflight.Qualification)error{
 if ctx.Err()!=nil{return ErrAuthorizationDenied}
 current,err:=a.qualify(ctx,op.ProposalID)
 if err!=nil||!matchesOperation(current,op,a.clock())||
  !sameEvidence(current,a.initial){return ErrAuthorizationDenied}
 return a.inner.Requalify(ctx,op,current)
}
func (a *alreadyAuthorizedEnvironment)PullImmutable(ctx context.Context,op executorjournal.Operation)error{return a.inner.PullImmutable(ctx,op)}
func (a *alreadyAuthorizedEnvironment)StageOverride(ctx context.Context,op executorjournal.Operation)error{return a.inner.StageOverride(ctx,op)}
func (a *alreadyAuthorizedEnvironment)ApplyTargetOnly(ctx context.Context,op executorjournal.Operation)error{return a.inner.ApplyTargetOnly(ctx,op)}
func (a *alreadyAuthorizedEnvironment)VerifyTargetAndSiblings(ctx context.Context,op executorjournal.Operation)error{return a.inner.VerifyTargetAndSiblings(ctx,op)}

func matchesOperation(q executorpreflight.Qualification,op executorjournal.Operation,now time.Time)bool{
 return q.ReadOnly&&q.ProposalID==op.ProposalID&&q.RegistrationID==op.RegistrationID&&
  q.Service==op.ServiceID&&q.CandidateDigest==op.CandidateDigest&&
  q.ProposalSHA256==op.ProposalSHA256&&sameDigest(q.ProposalSHA256)&&sameDigest(q.RegistrationSHA256)&&
  sameDigest(q.CandidateDigest)&&sameDigest(q.SourceSHA256)&&
  sameDigest(q.ImageID)&&sameDigest(q.PlatformManifestDigest)&&
  q.ContainerID!=""&&!q.CheckedAt.IsZero()&&!now.Before(q.CheckedAt)&&
  now.Sub(q.CheckedAt)<=MaxApplyEvidenceAge
}
func sameEvidence(a,b executorpreflight.Qualification)bool{
 return a.ProposalID==b.ProposalID&&a.ProposalSHA256==b.ProposalSHA256&&
  a.RegistrationID==b.RegistrationID&&a.RegistrationSHA256==b.RegistrationSHA256&&
  a.Service==b.Service&&a.CandidateDigest==b.CandidateDigest&&
  a.SourceSHA256==b.SourceSHA256&&
  a.PlatformManifestDigest==b.PlatformManifestDigest&&
  a.ContainerID==b.ContainerID&&a.ImageID==b.ImageID
}
func approveApply(ctx context.Context,op executorjournal.Operation,q executorpreflight.Qualification,tty Terminal,
 now func()time.Time,random func([]byte)(int,error))error{
 if ctx.Err()!=nil||!tty.Interactive()||!matchesOperation(q,op,now()){
  return ErrAuthorizationDenied
 }
 nonce:=make([]byte,16)
 n,err:=random(nonce)
 if err!=nil||n!=len(nonce){return ErrAuthorizationDenied}
 payload,err:=json.Marshal(struct{
  Operation,Project,Registration,RegistrationHash,Proposal,ProposalHash,Service,Candidate,Source,Container,Image,Manifest,Nonce string
 }{
  op.ID,op.ProjectID,q.RegistrationID,q.RegistrationSHA256,q.ProposalID,q.ProposalSHA256,q.Service,
  q.CandidateDigest,q.SourceSHA256,q.ContainerID,q.ImageID,
  q.PlatformManifestDigest,hex.EncodeToString(nonce),
 })
 if err!=nil{return ErrAuthorizationDenied}
 sum:=sha256.Sum256(payload)
 challenge:=hex.EncodeToString(sum[:6])
 // Only approved identifiers/digests; never print raw Compose, env,
 // proposal JSON, registry tokens, trusted host paths or Docker logs.
 message:=fmt.Sprintf("DOCKLANTERN — HOST-LOCAL APPLY CHALLENGE\nProject: %s\nService: %s\nCandidate: %s\nSource SHA-256: %s\nRegistration SHA-256: %s\nProposal SHA-256: %s\nActual container: %s\nType APPLY %s to authorize this exact candidate.\nThis action may replace the selected service.\n",
  op.ProjectID,q.Service,q.CandidateDigest,q.SourceSHA256,q.RegistrationSHA256,q.ProposalSHA256,q.ContainerID,challenge)
 if tty.Print(message)!=nil{return ErrAuthorizationDenied}
 started:=now()
 promptCtx,cancel:=context.WithTimeout(ctx,MaxApplyApproval)
 defer cancel()
 input,err:=tty.ReadLine(promptCtx)
 if err!=nil||promptCtx.Err()!=nil||input!="APPLY "+challenge||
  now().Before(started)||now().Sub(started)>MaxApplyApproval{
  return ErrAuthorizationDenied
 }
 return nil
}

// The default random reader is intentionally not coupled to the monitor.
// A future production entrypoint may inject rand.Read ONLY after establishing
// host-root and a controlling terminal.
func SecureChallengeRandom(p []byte)(int,error){return rand.Read(p)}
