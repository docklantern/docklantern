package executorpreflight

import (
 "context"
 "errors"
 "sync"

 "github.com/docklantern/docklantern/internal/executorregistration"
)

// ErrStagedEvidenceDenied never contains source bytes, names, credentials or
// subprocess output. Staging is read-only and conveys no Docker write authority.
var ErrStagedEvidenceDenied = errors.New("staged source evidence unavailable")

// stagedReadOnlyEvidence binds every Compose normalization in one qualification
// to the SAME verified root-private copy of the registered input bytes.
// It deliberately exposes only the ReadOnlyEvidence interface.
type stagedReadOnlyEvidence struct {
 delegate ReadOnlyEvidence
 stage func(executorregistration.Registration)(*SourceHandoff,error)
 mu sync.Mutex
 handoff *SourceHandoff
 registrationID string
 closed bool
}

func newStagedReadOnlyEvidence(delegate ReadOnlyEvidence,stage func(executorregistration.Registration)(*SourceHandoff,error))*stagedReadOnlyEvidence{
 return &stagedReadOnlyEvidence{delegate:delegate,stage:stage}
}

func (s *stagedReadOnlyEvidence) CaptureSources(ctx context.Context,reg executorregistration.Registration)(SourceSnapshot,error){
 var empty SourceSnapshot
 if ctx==nil||ctx.Err()!=nil||reg.Validate()!=nil||s==nil{
  return empty,ErrStagedEvidenceDenied
 }
 s.mu.Lock()
 defer s.mu.Unlock()
 if s.closed||s.delegate==nil||s.stage==nil{return empty,ErrStagedEvidenceDenied}
 if s.handoff==nil{
  h,err:=s.stage(reg)
  if err!=nil||h==nil{return empty,ErrStagedEvidenceDenied}
  s.handoff=h
  s.registrationID=reg.RegistrationID
 } else if s.registrationID!=reg.RegistrationID{
  return empty,ErrStagedEvidenceDenied
 }
 if s.handoff.RecheckOriginal(reg)!=nil{return empty,ErrStagedEvidenceDenied}
 return s.handoff.original,nil
}

func (s *stagedReadOnlyEvidence) NormalizedCompose(ctx context.Context,reg executorregistration.Registration,service,candidate string)([]byte,error){
 if ctx==nil||ctx.Err()!=nil||s==nil{return nil,ErrStagedEvidenceDenied}
 s.mu.Lock()
 defer s.mu.Unlock()
 if s.closed||s.handoff==nil||s.registrationID!=reg.RegistrationID{
  return nil,ErrStagedEvidenceDenied
 }
 // Only the production host adapter may render staged Compose configuration.
 // A mock must implement this same narrow method explicitly; the fallback
 // MUST NOT silently normalize mutable original files.
 renderer,ok:=s.delegate.(interface{
  NormalizedComposeFromHandoff(context.Context,executorregistration.Registration,*SourceHandoff,string,string)([]byte,error)
 })
 if !ok{return nil,ErrStagedEvidenceDenied}
 out,err:=renderer.NormalizedComposeFromHandoff(ctx,reg,s.handoff,service,candidate)
 if err!=nil||s.handoff.RecheckOriginal(reg)!=nil{return nil,ErrStagedEvidenceDenied}
 return out,nil
}

func (s *stagedReadOnlyEvidence) InspectActual(ctx context.Context,reg executorregistration.Registration,service string)(Actual,error){
 if s==nil||s.delegate==nil||ctx==nil||ctx.Err()!=nil{return Actual{},ErrStagedEvidenceDenied}
 s.mu.Lock()
 defer s.mu.Unlock()
 if s.closed||s.handoff==nil||s.registrationID!=reg.RegistrationID||
  s.handoff.RecheckOriginal(reg)!=nil{return Actual{},ErrStagedEvidenceDenied}
 actual,err:=s.delegate.InspectActual(ctx,reg,service)
 if err!=nil||s.handoff.RecheckOriginal(reg)!=nil{return Actual{},ErrStagedEvidenceDenied}
 return actual,nil
}

func (s *stagedReadOnlyEvidence) FetchOCI(ctx context.Context,repository,digest string)([]byte,[]byte,error){
 if s==nil||s.delegate==nil||ctx==nil||ctx.Err()!=nil{return nil,nil,ErrStagedEvidenceDenied}
 s.mu.Lock()
 defer s.mu.Unlock()
 if s.closed||s.handoff==nil{return nil,nil,ErrStagedEvidenceDenied}
 return s.delegate.FetchOCI(ctx,repository,digest)
}

// Close is called by the host CLI whether qualification succeeds or fails.
// Failure to clean up must prevent returning a successful qualification.
func (s *stagedReadOnlyEvidence) Close()error{
 if s==nil{return ErrStagedEvidenceDenied}
 s.mu.Lock()
 defer s.mu.Unlock()
 if s.closed{return ErrStagedEvidenceDenied}
 s.closed=true
 if s.handoff!=nil{
  if s.handoff.Close()!=nil{return ErrStagedEvidenceDenied}
  s.handoff=nil
 }
 return nil
}
