package executorapply

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "errors"
 "os"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executoroverride"
 "github.com/docklantern/docklantern/internal/executorpreflight"
 "github.com/docklantern/docklantern/internal/executorpreview"
 "github.com/docklantern/docklantern/internal/executorregistration"
 "github.com/docklantern/docklantern/internal/executorreview"
)

// HostJournalSession binds the protected SQLite journal's verified commit
// method to the same kernel project lock held by RunLockedCandidate.
type HostJournalSession struct {
 *executorjournal.Store
 lock *executorjournal.ProjectLock
 registration executorregistration.Registration
 previousDigest string
 hasPrevious bool
}
func (s *HostJournalSession)BindLease(lease ProjectLease)error{
 if s==nil||s.Store==nil||s.lock!=nil{return ErrAuthorizationDenied}
 lock,ok:=lease.(*executorjournal.ProjectLock)
 if !ok||lock==nil{return ErrAuthorizationDenied}
 s.lock=lock
 return nil
}
func (s *HostJournalSession)FinalizeVerified(ctx context.Context,id string,
 proof executorjournal.VerifiedEvidence,at time.Time)error{
 if s==nil||s.Store==nil||s.lock==nil{return ErrApplyUnavailable}
 if s.registration.PersistentOverride {
  // The public override must be durable before the protected success
  // transaction. A crash at this boundary leaves the journal's COMMIT
  // intent unresolved and blocks subsequent operations.
  if executoroverride.PublishVerified(s.registration,s.previousDigest,
   s.hasPrevious,proof.CandidateDigest)!=nil{return ErrApplyUnavailable}
 }
 if err:=s.Store.CommitVerified(ctx,s.lock,id,proof,at);err!=nil{return err}
 // The already-durable committed outcome must never be rolled back because
 // the separately mounted, nonauthoritative UI outbox is absent/unavailable.
 // Operators can inspect the protected journal if publication fails.
 _,_=s.Store.PublishReceipt(ctx,s.lock,id,at.Add(time.Nanosecond))
 return nil
}

var ErrLocalApplyDenied=errors.New("host-native APPLY denied or requires reconciliation")

// ApplyLocal is invoked only from the separately installed host-native
// executable. It accepts an opaque untrusted proposal ID, never arbitrary
// paths, image refs, CLI flags, web credentials or Docker commands.
// The caller must still separately authorize this VPS operation and have a
// verified off-host backup and restoration drill before running it.
func ApplyLocal(ctx context.Context,id string)(Result,error){
 var zero Result
 if ctx==nil||ctx.Err()!=nil||os.Geteuid()!=0||
  !executorregistration.ValidID(id){return zero,ErrLocalApplyDenied}
 tty,err:=executorreview.OpenLocalOperatorTerminal()
 if err!=nil{return zero,ErrLocalApplyDenied}
 defer tty.Close()
 uid,err:=executorpreview.LoadMonitorUID()
 if err!=nil{return zero,ErrLocalApplyDenied}
 source:=executorpreview.ProtectedSource{MonitorUID:uid}
 proposal,err:=source.LoadProposal(id,time.Now())
 if err!=nil{return zero,ErrLocalApplyDenied}
 reg,err:=source.LoadRegistration(proposal.Proposal.RegistrationID)
 if err!=nil||!proposal.Proposal.MatchesRegistration(reg.Registration){
  return zero,ErrLocalApplyDenied
 }
 nonce:=make([]byte,12)
 if n,e:=rand.Read(nonce);e!=nil||n!=len(nonce){return zero,ErrLocalApplyDenied}
 op:=executorjournal.Operation{
  ID:"apply-"+hex.EncodeToString(nonce),
  Kind:"apply",ProjectID:reg.Registration.ProjectName,
  RegistrationID:reg.Registration.RegistrationID,
  ServiceID:proposal.Proposal.Service,
  ProposalID:proposal.Proposal.ProposalID,
  ProposalSHA256:proposal.SHA256,
  CandidateDigest:proposal.Proposal.Candidate.Digest,
  CreatedAt:time.Now().UTC(),
 }
 var active *HostJournalSession
 d:=LockedDependencies{
  IsHostRoot:executorreview.HostRoot,
  AcquireLock:func(project string)(ProjectLease,error){
   return executorjournal.AcquireProjectLock(project)
  },
  OpenJournal:func()(JournalSession,error){
   store,e:=executorjournal.OpenHost()
   if e!=nil{return nil,e}
   session:=&HostJournalSession{Store:store,registration:reg.Registration}
   if reg.Registration.PersistentOverride {
    session.previousDigest,session.hasPrevious,e=store.LatestCommittedDesired(ctx,reg.Registration.ProjectName)
    if e!=nil{_ =store.Close();return nil,ErrLocalApplyDenied}
   }
   active=session
   return session,nil
  },
  Qualify:executorpreflight.QualifyLocal,
  Terminal:tty,Clock:time.Now,Random:SecureChallengeRandom,
  PrepareEnvironment:func(ctx context.Context,q executorpreflight.Qualification)(Environment,error){
   if ctx.Err()!=nil||q.ProposalSHA256!=proposal.SHA256||
    q.RegistrationSHA256!=reg.SHA256{return nil,ErrNativeEffect}
   // Reopen both protected authorities after the lock and fresh
   // qualification, rejecting even same-ID content replacement.
   latestProposal,e:=source.LoadProposal(id,time.Now())
   if e!=nil||latestProposal.SHA256!=proposal.SHA256{return nil,ErrNativeEffect}
   latestReg,e:=source.LoadRegistration(reg.Registration.RegistrationID)
   if e!=nil||latestReg.SHA256!=reg.SHA256{return nil,ErrNativeEffect}
   effects,e:=newHostEffects(latestReg.Registration,q,
    latestProposal.Proposal.Candidate.Repository,dockerNativeRunner{})
   if e!=nil{return nil,ErrNativeEffect}
   if latestReg.Registration.PersistentOverride {
    if active==nil||active.registration.RegistrationID!=latestReg.Registration.RegistrationID{
     return nil,ErrNativeEffect
    }
    effects.previousDigest=active.previousDigest
    effects.hasPrevious=active.hasPrevious
    if effects.checkPersistent()!=nil{return nil,ErrNativeEffect}
   }
   return effects,nil
  },
 }
 result,err:=RunLockedCandidate(ctx,op,d)
 if err!=nil{return result,ErrLocalApplyDenied}
 return result,nil
}
