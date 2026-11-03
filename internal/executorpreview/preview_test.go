package executorpreview

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/docklantern/docklantern/internal/executorproposal"
	"github.com/docklantern/docklantern/internal/executorregistration"
)

var fixtureTime = time.Date(2026, 11, 3, 5, 0, 0, 0, time.UTC)

type fakeSource struct {
	plan executorproposal.Record
	reg executorregistration.Record
	proposalReads int
	registrationReads int
	afterFirst func(*fakeSource)
}

func validFixture() *fakeSource {
	owner := uint32(1001)
	plan := executorproposal.Proposal{
		SchemaVersion: 1,
		ProposalID: "demo-plan", RegistrationID: "demo-registration", Service: "web",
		CreatedAt: fixtureTime.Add(-time.Minute).Format(time.RFC3339),
		ExpiresAt: fixtureTime.Add(10*time.Minute).Format(time.RFC3339),
		Candidate: executorproposal.Candidate{
			Repository: "docker.io/example/hello-go", RequestedTag: "latest",
			Digest: "sha256:" + strings.Repeat("a", 64),
			Platform: executorregistration.Platform{OS:"linux", Architecture:"amd64"},
		},
		Observed: executorproposal.Observed{
			ContainerID: strings.Repeat("b",64),
			ImageID: "sha256:"+strings.Repeat("c",64),
		},
	}
	reg := executorregistration.Registration{
		SchemaVersion: 1, RegistrationID: "demo-registration", ProjectName: "hello-go",
		ProjectDirectory: "/srv/deployments/hello-go",
		ComposeFiles: []string{"/srv/deployments/hello-go/compose.yaml"},
		TrustedSourceUID: &owner,
		Services: []executorregistration.Service{{
			Name: "web", DeploymentEnabled: true,
			ApprovedRepository: "docker.io/example/hello-go",
			Platform: executorregistration.Platform{OS:"linux", Architecture:"amd64"},
		}},
		FeaturePolicy: executorregistration.FeaturePolicyPrebuilt,
	}
	return &fakeSource{
		plan: executorproposal.Record{Proposal:plan, SHA256:"sha256:"+strings.Repeat("d",64)},
		reg: executorregistration.Record{Registration:reg, SHA256:"sha256:"+strings.Repeat("e",64)},
	}
}

func (s *fakeSource) LoadProposal(id string, now time.Time) (executorproposal.Record,error) {
	s.proposalReads++
	if s.afterFirst!=nil && s.proposalReads==2 {s.afterFirst(s)}
	if id!=s.plan.Proposal.ProposalID {return executorproposal.Record{}, ErrPreviewDenied}
	return s.plan,nil
}
func (s *fakeSource) LoadRegistration(id string) (executorregistration.Record,error) {
	s.registrationReads++
	if id!=s.reg.Registration.RegistrationID {return executorregistration.Record{},ErrPreviewDenied}
	return s.reg,nil
}

func TestPreviewRequiresTwoMatchingProtectedSnapshots(t *testing.T) {
	s:=validFixture()
	view,err:=Prepare(context.Background(),"demo-plan",s,func()time.Time{return fixtureTime})
	if err!=nil {t.Fatal(err)}
	if s.proposalReads!=2 || s.registrationReads!=2 {
		t.Fatalf("did not independently reopen both records: %d %d",s.proposalReads,s.registrationReads)
	}
	if view.Project!="hello-go" || view.Service!="web" ||
		view.CandidateDigest!=s.plan.Proposal.Candidate.Digest ||
		view.ProposalSHA256!=s.plan.SHA256 || view.RegistrationSHA256!=s.reg.SHA256 ||
		view.RunningOnlyAllowed {
		t.Fatalf("preview not bound to protected record: %#v",view)
	}
	text:=view.Render()
	for _,marker:=range []string{
		"READ ONLY", "demo-plan", "demo-registration","Project: hello-go",
		"docker.io/example/hello-go","linux/amd64","Requested tag (hint only)",
		"sha256:"+strings.Repeat("a",64),
		"Actual Docker/container baseline: NOT INSPECTED",
		"Effective Compose configuration: NOT INSPECTED",
		"OCI digest and platform authenticity: NOT VERIFIED",
		"DEPLOYMENT AUTHORIZATION: NOT GRANTED",
	} {
		if !strings.Contains(text,marker) {t.Errorf("preview missing %q",marker)}
	}
	if strings.Contains(text,"/srv/deployments") || strings.Contains(text,"container_id") {
		t.Fatal("preview disclosed unverified source path or Docker payload")
	}
}

func TestPreviewRejectsUnsafeAndChangedInputs(t *testing.T) {
	tests:=[]struct{
		name string
		mutate func(*fakeSource)
	}{
		{"disabled-service",func(s *fakeSource){s.reg.Registration.Services[0].DeploymentEnabled=false}},
		{"wrong-repository",func(s *fakeSource){s.reg.Registration.Services[0].ApprovedRepository="docker.io/other/hello"}},
		{"wrong-platform",func(s *fakeSource){s.plan.Proposal.Candidate.Platform.Architecture="arm64"}},
		{"forged-registration",func(s *fakeSource){s.plan.Proposal.RegistrationID="other"}},
		{"expired",func(s *fakeSource){s.plan.Proposal.ExpiresAt=fixtureTime.Add(-time.Second).Format(time.RFC3339)}},
		{"malformed-plan-hash",func(s *fakeSource){s.plan.SHA256="bad"}},
		{"malformed-registration-hash",func(s *fakeSource){s.reg.SHA256="bad"}},
		{"plan-replaced-mid-read",func(s *fakeSource){
			s.afterFirst=func(s *fakeSource){s.plan.SHA256="sha256:"+strings.Repeat("0",64)}
		}},
		{"registration-replaced-mid-read",func(s *fakeSource){
			s.afterFirst=func(s *fakeSource){s.reg.SHA256="sha256:"+strings.Repeat("0",64)}
		}},
	}
	for _,tc:=range tests{
		t.Run(tc.name,func(t *testing.T){
			s:=validFixture()
			tc.mutate(s)
			if _,err:=Prepare(context.Background(),"demo-plan",s,func()time.Time{return fixtureTime});!errors.Is(err,ErrPreviewDenied){
				t.Fatalf("unsafe preview accepted: %v",err)
			}
		})
	}
}

func TestPreviewExpiresBetweenReadsAndRejectsCanceledContext(t *testing.T) {
	s:=validFixture()
	clockCalls:=0
	clock:=func()time.Time{
		clockCalls++
		if clockCalls==1{return fixtureTime}
		return fixtureTime.Add(11*time.Minute)
	}
	if _,err:=Prepare(context.Background(),"demo-plan",s,clock);!errors.Is(err,ErrPreviewDenied){
		t.Fatalf("expired during preview was accepted: %v",err)
	}
	ctx,cancel:=context.WithCancel(context.Background())
	cancel()
	s=validFixture()
	if _,err:=Prepare(ctx,"demo-plan",s,func()time.Time{return fixtureTime});!errors.Is(err,ErrPreviewDenied) || s.proposalReads!=0 {
		t.Fatalf("canceled preview read private inputs: %v reads=%d",err,s.proposalReads)
	}
	if _,err:=Prepare(context.Background(),"../escape",s,func()time.Time{return fixtureTime});!errors.Is(err,ErrPreviewDenied) {
		t.Fatal("traversal proposal ID accepted")
	}
}

func TestPreviewRendersExplicitWeakerReadiness(t *testing.T) {
	s:=validFixture()
	s.reg.Registration.Services[0].AllowRunningOnly=true
	v,err:=Prepare(context.Background(),"demo-plan",s,func()time.Time{return fixtureTime})
	if err!=nil || !v.RunningOnlyAllowed || !strings.Contains(v.Render(),"weaker than healthy") {
		t.Fatalf("running-only policy not disclosed: %v %#v",err,v)
	}
}
