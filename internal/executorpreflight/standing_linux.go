package executorpreflight

import (
 "context"
 "os"
 "time"

 "github.com/docklantern/docklantern/internal/executorproposal"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

// QualifyStanding performs the exact same staged source, Compose, OCI and
// actual-Docker proof as an interactive preflight, using a typed in-memory
// host candidate instead of trusting a monitor-owned proposal file.
// The caller must independently enforce a protected standing authorization,
// check the registration's exact hash and hold the project lock.
func QualifyStanding(ctx context.Context, proposal executorproposal.Record,
 registration executorregistration.Record) (result Qualification, resultErr error) {
 if ctx == nil || ctx.Err() != nil || os.Geteuid() != 0 ||
  registration.Registration.AutomaticUpdates == nil ||
  registration.Registration.Validate() != nil ||
  proposal.Proposal.MatchesRegistration(registration.Registration) == false {
  return Qualification{}, ErrLocalQualificationUnavailable
 }
 reader, err := NewHostReadOnlyAdapter()
 if err != nil || reader.RequireSupportedCompose(ctx) != nil {
  return Qualification{}, ErrLocalQualificationUnavailable
 }
 staged := newStagedReadOnlyEvidence(reader, StageTrustedSources)
 defer func(){
  if staged.Close() != nil {
   result = Qualification{}
   resultErr = ErrLocalQualificationUnavailable
  }
 }()
 return Qualify(ctx, proposal, registration, staged, time.Now)
}
