package executorpreflight

import (
 "context"
 "errors"
 "time"

 "github.com/docklantern/docklantern/internal/executorproposal"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

// ErrQualificationDenied never includes untrusted Compose, registry, Docker,
// environment, or proposal contents in a caller-visible error.
var ErrQualificationDenied = errors.New("read-only deployment qualification denied")

const MaxActualAge = 10 * time.Second

// Actual is obtained independently from Docker/Compose, never from the
// monitor's Observed hint. SourceFingerprint is a hash of the effective
// source/configuration state, and is not a deployment authorization.
type Actual struct {
 ContainerID string
 ImageID string
 SourceFingerprint string
 CheckedAt time.Time
}

// ReadOnlyEvidence deliberately exposes NO subprocess method, Docker write
// command, approval capability, or mutation interface. Implementations must
// independently pin trusted local files and registry TLS/origin; fixtures in
// this PR provide deterministic, in-memory evidence only.
type ReadOnlyEvidence interface {
 CaptureSources(context.Context, executorregistration.Registration) (SourceSnapshot,error)
 NormalizedCompose(context.Context, executorregistration.Registration, string, string) ([]byte,error)
 InspectActual(context.Context, executorregistration.Registration, string) (Actual,error)
 FetchOCI(context.Context, string, string) ([]byte,[]byte,error)
}

// Qualification reports only a sanitized, immutable observation. It grants
// NO host-local approval and cannot be presented as an executable token.
type Qualification struct {
 ProposalID string
 ProposalSHA256 string
 RegistrationID string
 RegistrationSHA256 string
 Service string
 CandidateDigest string
 PlatformManifestDigest string
 SourceSHA256 string
 ContainerID string
 ImageID string
 CheckedAt time.Time
 ReadOnly bool
}

// Qualify ties together all Phase 4 *read-only* evidence in one fail-closed
// sequence. A second capture after OCI and Compose inspection rejects drift.
// A future host adapter must also create a stable source handoff across actual
// Compose execution, perform trusted registry retrieval and enforce feature
// policy. This pure coordinator does not implement those adapters.
func Qualify(
 ctx context.Context,
 proposal executorproposal.Record,
 registration executorregistration.Record,
 reader ReadOnlyEvidence,
 now func() time.Time,
) (Qualification,error) {
 var zero Qualification
 if ctx==nil||ctx.Err()!=nil||reader==nil||now==nil||
  registration.Registration.Validate()!=nil||
  proposal.Proposal.Validate(now())!=nil||
  proposal.Proposal.RegistrationID!=registration.Registration.RegistrationID||
  !proposal.Proposal.MatchesRegistration(registration.Registration)||
  !validDigest(proposal.SHA256)||!validDigest(registration.SHA256) {
  return zero,ErrQualificationDenied
 }
 plan:=proposal.Proposal
 reg:=registration.Registration
 first,err:=reader.CaptureSources(ctx,reg)
 if err!=nil||first.FileCount!=len(reg.ComposeFiles)+len(reg.EnvFiles)||
  !validDigest(first.SHA256){return zero,ErrQualificationDenied}
 actual,err:=reader.InspectActual(ctx,reg,plan.Service)
 if err!=nil||!validActual(actual,plan.Observed.ContainerID,plan.Observed.ImageID,now()){
  return zero,ErrQualificationDenied
 }
 before,err:=reader.NormalizedCompose(ctx,reg,plan.Service,"")
 if err!=nil{return zero,ErrQualificationDenied}
 candidateImage:=plan.Candidate.Repository+"@"+plan.Candidate.Digest
 after,err:=reader.NormalizedCompose(ctx,reg,plan.Service,candidateImage)
 if err!=nil||CheckImageOnly(before,after,reg,plan.Service,plan.Candidate.Repository,plan.Candidate.Digest)!=nil{
  return zero,ErrQualificationDenied
 }
 index,manifest,err:=reader.FetchOCI(ctx,plan.Candidate.Repository,plan.Candidate.Digest)
 if err!=nil{return zero,ErrQualificationDenied}
 proof,err:=VerifyOCIPlatformProof(index,manifest,plan.Candidate.Digest)
 if err!=nil||proof.OS!=plan.Candidate.Platform.OS||
  proof.Architecture!=plan.Candidate.Platform.Architecture{
  return zero,ErrQualificationDenied
 }
 // A fresh recheck AFTER the potentially expensive registry request catches
 // a changed source and actual container, including externally replaced
 // images. The future mutation path must repeat immediately before effects.
 second,err:=reader.CaptureSources(ctx,reg)
 if err!=nil||second!=first{return zero,ErrQualificationDenied}
 afterActual,err:=reader.InspectActual(ctx,reg,plan.Service)
 if err!=nil||!validActual(afterActual,plan.Observed.ContainerID,plan.Observed.ImageID,now())||
  afterActual.ContainerID!=actual.ContainerID||afterActual.ImageID!=actual.ImageID||
  afterActual.SourceFingerprint!=actual.SourceFingerprint {
  return zero,ErrQualificationDenied
 }
 beforeAgain,err:=reader.NormalizedCompose(ctx,reg,plan.Service,"")
 if err!=nil||!sameBytes(beforeAgain,before){return zero,ErrQualificationDenied}
 afterAgain,err:=reader.NormalizedCompose(ctx,reg,plan.Service,candidateImage)
 if err!=nil||!sameBytes(afterAgain,after){return zero,ErrQualificationDenied}
 if ctx.Err()!=nil||proposal.Proposal.Validate(now())!=nil{return zero,ErrQualificationDenied}
 return Qualification{
  ProposalID:plan.ProposalID,ProposalSHA256:proposal.SHA256,
  RegistrationID:reg.RegistrationID,RegistrationSHA256:registration.SHA256,Service:plan.Service,
  CandidateDigest:plan.Candidate.Digest,
  PlatformManifestDigest:proof.PlatformManifestDigest,
  SourceSHA256:second.SHA256,ContainerID:afterActual.ContainerID,
  ImageID:afterActual.ImageID,CheckedAt:now(),ReadOnly:true,
 },nil
}

func sameBytes(a,b []byte)bool{
 if len(a)!=len(b){return false}
 for i:=range a{if a[i]!=b[i]{return false}}
 return true
}

func validActual(a Actual,expectedContainer,expectedImage string,at time.Time)bool{
 return a.ContainerID!=""&&a.ContainerID==expectedContainer&&
  a.ImageID==expectedImage&&validDigest(a.ImageID)&&
  validDigest(a.SourceFingerprint)&&!a.CheckedAt.IsZero()&&
  !a.CheckedAt.After(at)&&at.Sub(a.CheckedAt)<=MaxActualAge
}
