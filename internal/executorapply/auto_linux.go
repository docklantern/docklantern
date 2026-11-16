package executorapply

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "os"
 "time"

 "github.com/docklantern/docklantern/internal/executorjournal"
 "github.com/docklantern/docklantern/internal/executoroverride"
 "github.com/docklantern/docklantern/internal/executorpreflight"
 "github.com/docklantern/docklantern/internal/executorproposal"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrAutomaticUpdate = errors.New("automatic host update unavailable or requires manual reconciliation")

// AutoOnce is invoked only by an independently installed, root-owned host
// systemd timer. The monitor has no executor entrypoint or Docker write path.
// The registration must be explicitly authorized through the original
// root-plus-TTY register command, including an exact repository and tag.
func AutoOnce(ctx context.Context, registrationID string) (Result,error) {
 var zero Result
 if os.Geteuid()!=0 || ctx==nil || ctx.Err()!=nil ||
  !executorregistration.ValidID(registrationID) {return zero,ErrAutomaticUpdate}
 reg,err:=executorregistration.LoadProtected(registrationID)
 if err!=nil || !standingRegistration(reg.Registration) {
  return zero,ErrAutomaticUpdate
 }
 service:=reg.Registration.Services[0]
 digest,err:=executorpreflight.ResolvePublicTag(ctx,service.ApprovedRepository,
  reg.Registration.AutomaticUpdates.Tag)
 if err!=nil{return zero,ErrAutomaticUpdate}
 reader,err:=executorpreflight.NewHostReadOnlyAdapter()
 if err!=nil{return zero,ErrAutomaticUpdate}
 actual,err:=reader.InspectActual(ctx,reg.Registration,service.Name)
 if err!=nil{return zero,ErrAutomaticUpdate}
 nonce:=make([]byte,12)
 if n,e:=rand.Read(nonce);e!=nil||n!=len(nonce){return zero,ErrAutomaticUpdate}
 now:=time.Now().UTC()
 proposal:=executorproposal.Proposal{
  SchemaVersion:executorproposal.SchemaVersion,
  ProposalID:"auto-"+hex.EncodeToString(nonce),
  CreatedAt:now.Format(time.RFC3339Nano),
  ExpiresAt:now.Add(10*time.Minute).Format(time.RFC3339Nano),
  RegistrationID:registrationID,
  Service:service.Name,
  Candidate:executorproposal.Candidate{
   Repository:service.ApprovedRepository,
   RequestedTag:reg.Registration.AutomaticUpdates.Tag,
   Digest:digest,Platform:service.Platform,
  },
  Observed:executorproposal.Observed{ContainerID:actual.ContainerID,ImageID:actual.ImageID},
 }
 raw,err:=json.Marshal(proposal)
 if err!=nil{return zero,ErrAutomaticUpdate}
 plan,err:=executorproposal.Parse(raw,now)
 if err!=nil||!plan.Proposal.MatchesRegistration(reg.Registration){
  return zero,ErrAutomaticUpdate
 }
 if n,e:=rand.Read(nonce);e!=nil||n!=len(nonce){return zero,ErrAutomaticUpdate}
 op:=executorjournal.Operation{
  ID:"apply-"+hex.EncodeToString(nonce),Kind:"apply",
  ProjectID:reg.Registration.ProjectName,
  RegistrationID:registrationID,ServiceID:service.Name,
  ProposalID:plan.Proposal.ProposalID,ProposalSHA256:plan.SHA256,
  CandidateDigest:digest,CreatedAt:now,
 }

 var session *HostJournalSession
 // Every callback is run under the same project lock. Never use mutable
 // registry hints or a monitor-owned file as a standing authorization.
 checkPolicy:=func(ctx context.Context)error{
  if ctx.Err()!=nil{return ErrAutomaticUpdate}
  latest,e:=executorregistration.LoadProtected(registrationID)
  if e!=nil||latest.SHA256!=reg.SHA256||!standingRegistration(latest.Registration){
   return ErrAutomaticUpdate
  }
  remote,e:=executorpreflight.ResolvePublicTag(ctx,service.ApprovedRepository,
   reg.Registration.AutomaticUpdates.Tag)
  if e!=nil||remote!=digest{return ErrAutomaticUpdate}
  return nil
 }
 deps:=StandingDependencies{
  IsHostRoot:func()bool{return os.Geteuid()==0},
  AcquireLock:func(project string)(ProjectLease,error){
   return executorjournal.AcquireProjectLock(project)
  },
  OpenJournal:func()(JournalSession,error){
   store,e:=executorjournal.OpenHost()
   if e!=nil{return nil,e}
   s:=&HostJournalSession{Store:store,registration:reg.Registration}
   s.previousDigest,s.hasPrevious,e=store.LatestCommittedDesired(ctx,reg.Registration.ProjectName)
   if e!=nil{_ =store.Close();return nil,ErrAutomaticUpdate}
   session=s
   return s,nil
  },
  CheckStanding:checkPolicy,
  NoChange:func(ctx context.Context)(bool,error){
   if session==nil||!standingRegistration(session.registration) ||
    executoroverride.VerifyCurrent(reg.Registration,session.previousDigest,session.hasPrevious)!=nil{
    return false,ErrAutomaticUpdate
   }
   blocked,e:=session.Store.HasUnresolvedProject(ctx,reg.Registration.ProjectName)
   if e!=nil||blocked{return false,ErrAutomaticUpdate}
   if !session.hasPrevious||session.previousDigest!=digest{return false,nil}
   // The tag did not move, but a Compose override alone does not prove
   // that the service is still running the committed candidate.
   effects:=&HostEffects{reg:reg.Registration,service:service.Name,
    repository:service.ApprovedRepository,digest:digest,runner:dockerNativeRunner{}}
   containers,e:=effects.captureProject(ctx)
   if e!=nil{return false,ErrAutomaticUpdate}
   found:=0
   for _,container:=range containers{
    if container.Config.Labels["com.docker.compose.service"]==service.Name{
     found++
     if !container.State.Running ||
      container.ID!=actual.ContainerID || container.Image!=actual.ImageID ||
      container.Config.ImageReference!=service.ApprovedRepository+"@"+digest{
      return false,ErrAutomaticUpdate
     }
     if container.State.Health!=nil {
      if container.State.Health.Status!="healthy"{return false,ErrAutomaticUpdate}
     }else if !service.AllowRunningOnly{return false,ErrAutomaticUpdate}
    }
   }
   return found==1,nil
  },
  Qualify:func(ctx context.Context,id string)(executorpreflight.Qualification,error){
   if id!=plan.Proposal.ProposalID||ctx.Err()!=nil{return executorpreflight.Qualification{},ErrAutomaticUpdate}
   latest,e:=executorregistration.LoadProtected(registrationID)
   if e!=nil||latest.SHA256!=reg.SHA256{return executorpreflight.Qualification{},ErrAutomaticUpdate}
   return executorpreflight.QualifyStanding(ctx,plan,latest)
  },
  PrepareEnvironment:func(ctx context.Context,q executorpreflight.Qualification)(Environment,error){
   if ctx.Err()!=nil||session==nil||q.ProposalSHA256!=plan.SHA256||
    q.RegistrationSHA256!=reg.SHA256{return nil,ErrAutomaticUpdate}
   latest,e:=executorregistration.LoadProtected(registrationID)
   if e!=nil||latest.SHA256!=reg.SHA256{return nil,ErrAutomaticUpdate}
   effects,e:=newHostEffects(latest.Registration,q,service.ApprovedRepository,dockerNativeRunner{})
   if e!=nil{return nil,ErrAutomaticUpdate}
   effects.previousDigest=session.previousDigest
   effects.hasPrevious=session.hasPrevious
   if effects.checkPersistent()!=nil{return nil,ErrAutomaticUpdate}
   return effects,nil
  },
  Clock:time.Now,
 }
 result,err:=RunLockedStandingCandidate(ctx,op,deps)
 if err!=nil||result.NeedsReconciliation{return result,ErrAutomaticUpdate}
 return result,nil
}

func standingRegistration(r executorregistration.Registration)bool{
 return r.Validate()==nil&&r.PersistentOverride&&r.AutomaticUpdates!=nil&&
  r.AutomaticUpdates.Enabled&&len(r.Services)==1&&r.Services[0].DeploymentEnabled&&
  r.ProjectName!="docklantern"&&r.ProjectName!="traefik"&&
  r.ProjectName!="docker-socket-proxy"&&r.Services[0].Name!="docklantern"&&
  r.Services[0].Name!="traefik"&&r.Services[0].Name!="docker-socket-proxy"
}
