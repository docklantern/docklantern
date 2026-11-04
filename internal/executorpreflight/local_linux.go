package executorpreflight

import (
 "context"
 "errors"
 "os"
 "syscall"
 "time"

 "github.com/docklantern/docklantern/internal/executorpreview"
 "github.com/docklantern/docklantern/internal/executorproposal"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrLocalQualificationUnavailable = errors.New("protected local qualification unavailable")

// QualificationSource is a narrow, read-only seam for hermetic CI. Production
// uses protected host readers and never accepts paths or untrusted monitor UIDs.
type QualificationSource interface {
 LoadProposal(string,time.Time)(executorproposal.Record,error)
 LoadRegistration(string)(executorregistration.Record,error)
}

// QualifyFromSource is a read-only coordinator: it never writes a journal,
// accepts confirmation, enables a Docker mutation or grants an execution token.
func QualifyFromSource(ctx context.Context,id string,source QualificationSource,
 reader ReadOnlyEvidence,now func()time.Time)(Qualification,error) {
 var zero Qualification
 if ctx==nil||ctx.Err()!=nil||source==nil||reader==nil||now==nil||
  !executorregistration.ValidID(id){return zero,ErrLocalQualificationUnavailable}
 first,err:=source.LoadProposal(id,now())
 if err!=nil||first.Proposal.ProposalID!=id{return zero,ErrLocalQualificationUnavailable}
 reg,err:=source.LoadRegistration(first.Proposal.RegistrationID)
 if err!=nil||reg.Registration.RegistrationID!=first.Proposal.RegistrationID{
  return zero,ErrLocalQualificationUnavailable
 }
 qualified,err:=Qualify(ctx,first,reg,reader,now)
 if err!=nil{return zero,ErrLocalQualificationUnavailable}
 // Reopen both protected records AFTER the entire Compose/OCI/Docker check.
 // Exact-byte hashes and plan validity must survive all prior async reads.
 again,err:=source.LoadProposal(id,now())
 if err!=nil||again.SHA256!=first.SHA256||
  again.Proposal.Validate(now())!=nil{return zero,ErrLocalQualificationUnavailable}
 againReg,err:=source.LoadRegistration(first.Proposal.RegistrationID)
 if err!=nil||againReg.SHA256!=reg.SHA256||
  againReg.Registration.Validate()!=nil||ctx.Err()!=nil{
  return zero,ErrLocalQualificationUnavailable
 }
 return qualified,nil
}

// QualifyLocal is a host-root, controlling-TTY-only *read-only* command.
// No monitoring web process can call it, and no Docker operation is written.
// A positive report remains explicitly non-authorizing.
func QualifyLocal(ctx context.Context,id string)(result Qualification, resultErr error) {
 var zero Qualification
 if os.Geteuid()!=0||ctx==nil||ctx.Err()!=nil{
  return zero,ErrLocalQualificationUnavailable
 }
 tty,err:=os.OpenFile("/dev/tty",os.O_RDONLY|syscall.O_CLOEXEC,0)
 if err!=nil{return zero,ErrLocalQualificationUnavailable}
 st,err:=tty.Stat()
 _=tty.Close()
 if err!=nil||st.Mode()&os.ModeCharDevice==0{
  return zero,ErrLocalQualificationUnavailable
 }
 uid,err:=executorpreview.LoadMonitorUID()
 if err!=nil{return zero,ErrLocalQualificationUnavailable}
 reader,err:=NewHostReadOnlyAdapter()
 if err!=nil||reader.RequireSupportedCompose(ctx)!=nil{return zero,ErrLocalQualificationUnavailable}
 // Use one trusted staged copy for every Compose read in the entire
 // qualification. The stage is unconditionally destroyed before success
 // can be reported; a cleanup failure itself denies qualification.
 staged:=newStagedReadOnlyEvidence(reader,StageTrustedSources)
 defer func(){
  if staged.Close()!=nil{
   result=Qualification{}
   resultErr=ErrLocalQualificationUnavailable
  }
 }()
 return QualifyFromSource(ctx,id,executorpreview.ProtectedSource{MonitorUID:uid},staged,time.Now)
}
