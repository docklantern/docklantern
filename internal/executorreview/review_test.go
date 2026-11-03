package executorreview

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/docklantern/docklantern/internal/executorproposal"
	"github.com/docklantern/docklantern/internal/executorregistration"
)

var reviewTime = time.Date(2026, 11, 3, 2, 0, 0, 0, time.UTC)

type simulatedSource struct {
	proposal executorproposal.Record
	registration executorregistration.Record
	actual Baseline
	proposalReads int
	registrationReads int
	actualReads int
	broken bool
}

func fixtureReviewSource() *simulatedSource {
	owner := uint32(1001)
	p := executorproposal.Proposal{
		SchemaVersion: executorproposal.SchemaVersion,
		ProposalID: "plan-demo",
		RegistrationID: "hello-demo",
		Service: "web",
		CreatedAt: reviewTime.Add(-time.Minute).Format(time.RFC3339),
		ExpiresAt: reviewTime.Add(10*time.Minute).Format(time.RFC3339),
		Candidate: executorproposal.Candidate{
			Repository: "docker.io/example/hello",
			RequestedTag: "latest",
			Digest: "sha256:" + strings.Repeat("c", 64),
			Platform: executorregistration.Platform{OS: "linux", Architecture: "amd64"},
		},
		Observed: executorproposal.Observed{
			ContainerID: strings.Repeat("a", 64),
			ImageID: "sha256:" + strings.Repeat("b", 64),
		},
	}
	r := executorregistration.Registration{
		SchemaVersion: executorregistration.SchemaVersion,
		RegistrationID: "hello-demo", ProjectName: "hello",
		ProjectDirectory: "/srv/apps/hello",
		ComposeFiles: []string{"/srv/apps/hello/compose.yaml"},
		TrustedSourceUID: &owner,
		Services: []executorregistration.Service{{
			Name: "web", DeploymentEnabled: true, ApprovedRepository: "docker.io/example/hello",
			Platform: executorregistration.Platform{OS: "linux", Architecture: "amd64"},
			AllowRunningOnly: true,
		}},
		FeaturePolicy: executorregistration.FeaturePolicyPrebuilt,
	}
	return &simulatedSource{
		proposal: executorproposal.Record{Proposal: p, SHA256: "sha256:"+strings.Repeat("d",64)},
		registration: executorregistration.Record{Registration: r, SHA256: "sha256:"+strings.Repeat("e",64)},
		actual: Baseline{ContainerID: p.Observed.ContainerID, ImageID: p.Observed.ImageID,
			SourceFingerprint: "sha256:"+strings.Repeat("f",64), CheckedAt: reviewTime.Add(-time.Second)},
	}
}
func (s *simulatedSource) LoadProposal(id string, now time.Time) (executorproposal.Record, error) {
	s.proposalReads++
	if s.broken { return executorproposal.Record{}, errors.New("secret: should not leak") }
	return s.proposal, nil
}
func (s *simulatedSource) LoadRegistration(id string) (executorregistration.Record, error) {
	s.registrationReads++
	return s.registration, nil
}
func (s *simulatedSource) InspectActual(ctx context.Context, reg executorregistration.Record, svc string) (Baseline, error) {
	s.actualReads++
	if svc != "web" { return Baseline{}, ErrReviewDenied }
	return s.actual, nil
}

type simulatedTTY struct {
	interactive bool
	text string
	answer string
	onRead func()
	reads int
}
var tokenRE = regexp.MustCompile(`Type REVIEW ([A-F0-9]{12}) to acknowledge`)
func (t *simulatedTTY) Interactive() bool { return t.interactive }
func (t *simulatedTTY) Print(s string) error {
	t.text = s
	tokens := tokenRE.FindStringSubmatch(s)
	if len(tokens)==2 { t.answer = "REVIEW "+tokens[1] }
	return nil
}
func (t *simulatedTTY) ReadLine(ctx context.Context) (string, error) {
	t.reads++
	if t.onRead != nil { t.onRead() }
	return t.answer, nil
}

func TestReviewBindsExactPlanRegistrationActualAndExpiry(t *testing.T) {
	src, tty := fixtureReviewSource(), &simulatedTTY{interactive: true}
	r, err := Review(context.Background(), "plan-demo", src, tty, func()bool{return true}, func()time.Time{return reviewTime})
	if err != nil { t.Fatal(err) }
	if !r.ReviewOnly || r.ProposalID != "plan-demo" || r.CandidateDigest != src.proposal.Proposal.Candidate.Digest ||
		r.ProposalSHA256 != src.proposal.SHA256 || r.RegistrationSHA256 != src.registration.SHA256 ||
		!strings.HasPrefix(r.BaselineSHA256,"sha256:") {
		t.Fatalf("unbound review receipt %#v", r)
	}
	for _, marker := range []string{
		"READ-ONLY", "NOT DEPLOYMENT APPROVAL", "hello-demo", "Project: hello", "Service: web",
		"docker.io/example/hello", "sha256:"+strings.Repeat("c",64),
		"linux/amd64", "2026-11-03T02:10:00Z",
		"sha256:"+strings.Repeat("d",64), "sha256:"+strings.Repeat("e",64),
		"Running-only verification permitted", "NO DOCKER MUTATION",
	} {
		if !strings.Contains(tty.text, marker) { t.Errorf("preview missing %q",marker) }
	}
	if src.proposalReads != 2 || src.registrationReads != 2 || src.actualReads != 2 || tty.reads != 1 {
		t.Fatalf("must independently re-read after confirmation: proposal=%d registration=%d actual=%d tty=%d",
			src.proposalReads,src.registrationReads,src.actualReads,tty.reads)
	}
}

func TestReviewRejectsNonRootNonTTYInvalidIdOrCanceled(t *testing.T) {
	for _, tc := range []struct{
		name, id string; admin, tty bool; cancel bool
	}{
		{"nonroot","plan-demo",false,true,false},
		{"nonTTY","plan-demo",true,false,false},
		{"path","../escape",true,true,false},
		{"canceled","plan-demo",true,true,true},
	} {
		t.Run(tc.name,func(t *testing.T) {
			s, terminal := fixtureReviewSource(), &simulatedTTY{interactive:tc.tty}
			ctx,cancel := context.WithCancel(context.Background())
			if tc.cancel { cancel() } else { defer cancel() }
			if _,err:=Review(ctx,tc.id,s,terminal,func()bool{return tc.admin},func()time.Time{return reviewTime}); !errors.Is(err,ErrReviewDenied) {
				t.Fatalf("unauthorized review allowed: %v",err)
			}
			if s.proposalReads!=0 || terminal.reads!=0 { t.Fatal("unprivileged review accessed untrusted state") }
		})
	}
}

func TestReviewRejectsMissingOrUnqualifiedBaseline(t *testing.T) {
	cases:=[]struct{name string; change func(*simulatedSource)}{
		{"image-mismatch",func(s *simulatedSource){s.actual.ImageID="sha256:"+strings.Repeat("0",64)}},
		{"container-mismatch",func(s *simulatedSource){s.actual.ContainerID=strings.Repeat("0",64)}},
		{"stale",func(s *simulatedSource){s.actual.CheckedAt=reviewTime.Add(-11*time.Second)}},
		{"future",func(s *simulatedSource){s.actual.CheckedAt=reviewTime.Add(time.Second)}},
		{"missing-source",func(s *simulatedSource){s.actual.SourceFingerprint=""}},
		{"disabled-service",func(s *simulatedSource){s.registration.Registration.Services[0].DeploymentEnabled=false}},
		{"unauthorized-repository",func(s *simulatedSource){s.registration.Registration.Services[0].ApprovedRepository="docker.io/elsewhere/hello"}},
		{"forged-reg",func(s *simulatedSource){s.registration.Registration.RegistrationID="other"}},
		{"expired",func(s *simulatedSource){s.proposal.Proposal.ExpiresAt=reviewTime.Add(-time.Second).Format(time.RFC3339)}},
		{"malformed-hash",func(s *simulatedSource){s.proposal.SHA256="bad"}},
		{"unreadable",func(s *simulatedSource){s.broken=true}},
	}
	for _,tc:=range cases{
		t.Run(tc.name,func(t *testing.T){
			src,tty:=fixtureReviewSource(),&simulatedTTY{interactive:true}
			tc.change(src)
			_,err:=Review(context.Background(),"plan-demo",src,tty,func()bool{return true},func()time.Time{return reviewTime})
			if !errors.Is(err,ErrReviewDenied)||tty.reads!=0 {
				t.Fatalf("unqualified input reached TTY: %v",err)
			}
		})
	}
}

func TestReviewRejectsReplayedOrEditedAfterTTYPhrase(t *testing.T) {
	cases:=[]struct{name string; mutate func(*simulatedSource,*simulatedTTY)}{
		{"wrong-response",func(_ *simulatedSource,t *simulatedTTY){t.answer="YES"}},
		{"changed-proposal-hash",func(s *simulatedSource,_ *simulatedTTY){s.proposal.SHA256="sha256:"+strings.Repeat("0",64)}},
		{"changed-registration",func(s *simulatedSource,_ *simulatedTTY){s.registration.SHA256="sha256:"+strings.Repeat("0",64)}},
		{"changed-actual",func(s *simulatedSource,_ *simulatedTTY){s.actual.SourceFingerprint="sha256:"+strings.Repeat("0",64)}},
		{"changed-image",func(s *simulatedSource,_ *simulatedTTY){s.actual.ImageID="sha256:"+strings.Repeat("0",64)}},
	}
	for _,tc:=range cases {
		t.Run(tc.name,func(t *testing.T) {
			s,tty:=fixtureReviewSource(),&simulatedTTY{interactive:true}
			tty.onRead=func(){tc.mutate(s,tty)}
			_,err:=Review(context.Background(),"plan-demo",s,tty,func()bool{return true},func()time.Time{return reviewTime})
			if !errors.Is(err,ErrReviewDenied) {t.Fatal("stale/edited snapshot acknowledged")}
		})
	}
}

func TestReviewFailsIfExpiresWhileTyping(t *testing.T) {
	s,tty:=fixtureReviewSource(),&simulatedTTY{interactive:true}
	current:=reviewTime
	tty.onRead=func(){current=reviewTime.Add(11*time.Minute)}
	_,err:=Review(context.Background(),"plan-demo",s,tty,func()bool{return true},func()time.Time{return current})
	if !errors.Is(err,ErrReviewDenied) {t.Fatalf("expired plan acknowledged: %v",err)}
}

func TestReviewCannotUseStaleTokenFromPriorRun(t *testing.T) {
	src,tty:=fixtureReviewSource(),&simulatedTTY{interactive:true}
	if _,err:=Review(context.Background(),"plan-demo",src,tty,func()bool{return true},func()time.Time{return reviewTime});err!=nil {t.Fatal(err)}
	old:=tty.answer
	src2,tty2:=fixtureReviewSource(),&simulatedTTY{interactive:true}
	tty2.onRead=func(){tty2.answer=old}
	if _,err:=Review(context.Background(),"plan-demo",src2,tty2,func()bool{return true},func()time.Time{return reviewTime});!errors.Is(err,ErrReviewDenied) {t.Fatal("replayed token acknowledged")}
}
