package executorpreflight

import (
 "context"
 "errors"
 "testing"
 "strings"
 "time"

 "github.com/docklantern/docklantern/internal/executorproposal"
 "github.com/docklantern/docklantern/internal/executorregistration"
)

type qualificationFixture struct{
 reg executorregistration.Record
 proposal executorproposal.Record
 reader *qualificationReader
 now time.Time
}
type qualificationReader struct{
 source SourceSnapshot
 actual Actual
 before,after,index,manifest []byte
 sourceCalls,actualCalls,composeCalls int
 driftSource,driftActual,driftCompose bool
 registryError bool
}
func (r *qualificationReader) CaptureSources(_ context.Context,_ executorregistration.Registration)(SourceSnapshot,error){
 r.sourceCalls++
 if r.driftSource&&r.sourceCalls>1{return SourceSnapshot{SHA256:"sha256:"+strings.Repeat("f",64),FileCount:r.source.FileCount},nil}
 return r.source,nil
}
func (r *qualificationReader) NormalizedCompose(_ context.Context,_ executorregistration.Registration,_ string,candidate string)([]byte,error){
 r.composeCalls++
 if candidate==""{
  if r.driftCompose&&r.composeCalls>2{return append(append([]byte(nil),r.before...),byte(' ')),nil}
  return r.before,nil
 }
 return r.after,nil
}
func (r *qualificationReader) InspectActual(_ context.Context,_ executorregistration.Registration,_ string)(Actual,error){
 r.actualCalls++
 if r.driftActual&&r.actualCalls>1{
  changed:=r.actual
  changed.ImageID="sha256:"+strings.Repeat("e",64)
  return changed,nil
 }
 return r.actual,nil
}
func (r *qualificationReader) FetchOCI(_ context.Context,_,_ string)([]byte,[]byte,error){
 if r.registryError{return nil,nil,errors.New("untrusted registry unavailable")}
 return r.index,r.manifest,nil
}
func makeQualificationFixture()qualificationFixture{
 clock:=time.Date(2026,11,4,0,0,0,0,time.UTC)
 reg:=fixtureRegistration()
 ib,mb,digest:=ociFixture()
 before,after,_:=configs()
 // The candidate in the index fixture is a real SHA-256 digest, not the
 // synthetic digest used in the simpler image-only unit tests.
 after["services"].(map[string]any)["web"].(map[string]any)["image"]="docker.io/example/hello-go@"+digest
 proposal:=executorproposal.Proposal{
  SchemaVersion:executorproposal.SchemaVersion,ProposalID:"proposal-demo",
  RegistrationID:reg.RegistrationID,Service:"web",
  CreatedAt:clock.Add(-time.Minute).Format(time.RFC3339Nano),
  ExpiresAt:clock.Add(time.Minute).Format(time.RFC3339Nano),
  Candidate:executorproposal.Candidate{Repository:"docker.io/example/hello-go",
   RequestedTag:"stable",Digest:digest,
   Platform:executorregistration.Platform{OS:"linux",Architecture:"amd64"}},
  Observed:executorproposal.Observed{
   ContainerID:strings.Repeat("a",64),ImageID:"sha256:"+strings.Repeat("b",64)},
 }
 return qualificationFixture{
  reg:executorregistration.Record{Registration:reg,SHA256:"sha256:"+strings.Repeat("c",64)},
  proposal:executorproposal.Record{Proposal:proposal,SHA256:"sha256:"+strings.Repeat("d",64)},
  reader:&qualificationReader{
   source:SourceSnapshot{SHA256:"sha256:"+strings.Repeat("a",64),FileCount:1},
   actual:Actual{ContainerID:proposal.Observed.ContainerID,ImageID:proposal.Observed.ImageID,
    SourceFingerprint:"sha256:"+strings.Repeat("c",64),CheckedAt:clock.Add(-time.Second)},
   before:raw(before),after:raw(after),index:ib,manifest:mb,
  },
  now:clock,
 }
}
func (f qualificationFixture) run()(Qualification,error){
 return Qualify(context.Background(),f.proposal,f.reg,f.reader,func()time.Time{return f.now})
}
func TestQualifyReadOnlyFullEvidence(t *testing.T){
 f:=makeQualificationFixture()
 result,err:=f.run()
 if err!=nil||!result.ReadOnly||result.CandidateDigest!=f.proposal.Proposal.Candidate.Digest||
  result.SourceSHA256!=f.reader.source.SHA256||f.reader.sourceCalls!=2||
  f.reader.actualCalls!=2||f.reader.composeCalls!=4 {
  t.Fatalf("complete evidence not qualified: %#v %v",result,err)
 }
}
func TestQualifyRejectsDriftAndUntrustedEvidence(t *testing.T){
 tests:=[]struct{name string;change func(*qualificationFixture)}{
  {"source_drift",func(f *qualificationFixture){f.reader.driftSource=true}},
  {"docker_drift",func(f *qualificationFixture){f.reader.driftActual=true}},
  {"compose_drift",func(f *qualificationFixture){f.reader.driftCompose=true}},
  {"registry_unavailable",func(f *qualificationFixture){f.reader.registryError=true}},
  {"stale_baseline",func(f *qualificationFixture){f.reader.actual.CheckedAt=f.now.Add(-MaxActualAge-time.Second)}},
  {"future_baseline",func(f *qualificationFixture){f.reader.actual.CheckedAt=f.now.Add(time.Second)}},
  {"unapproved_service",func(f *qualificationFixture){f.proposal.Proposal.Service="worker"}},
  {"changed_monitor_hint",func(f *qualificationFixture){f.proposal.Proposal.Observed.ImageID="sha256:"+strings.Repeat("f",64)}},
  {"expired_plan",func(f *qualificationFixture){f.now=f.now.Add(2*time.Minute)}},
  {"wrong_platform",func(f *qualificationFixture){f.proposal.Proposal.Candidate.Platform.Architecture="arm64"}},
  {"tampered_registry_index",func(f *qualificationFixture){f.reader.index=append(f.reader.index,' ')}},
  {"invalid_registration",func(f *qualificationFixture){f.reg.Registration.Services[0].DeploymentEnabled=false}},
  {"sibling_modified",func(f *qualificationFixture){
   f.reader.after=[]byte(strings.ReplaceAll(string(f.reader.after),"docker.io/example/worker:latest","docker.io/example/worker:other"))
  }},
  {"wrong_source_count",func(f *qualificationFixture){f.reader.source.FileCount=2}},
 }
 for _,tc:=range tests{t.Run(tc.name,func(t *testing.T){
  f:=makeQualificationFixture()
  tc.change(&f)
  result,err:=f.run()
  if !errors.Is(err,ErrQualificationDenied)||result.ReadOnly {
   t.Fatalf("unsafe qualification was accepted: %#v %v",result,err)
  }
 })}
}
