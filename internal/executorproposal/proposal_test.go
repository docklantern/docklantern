package executorproposal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/docklantern/docklantern/internal/executorregistration"
)

var sampleNow = time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC)

func sampleProposal() Proposal {
	return Proposal{
		SchemaVersion: SchemaVersion,
		ProposalID: "plan-example-01", RegistrationID: "hello-go-demo",
		CreatedAt: sampleNow.Add(-time.Minute).Format(time.RFC3339),
		ExpiresAt: sampleNow.Add(10*time.Minute).Format(time.RFC3339),
		Service: "web",
		Candidate: Candidate{
			Repository: "docker.io/example/hello-go", RequestedTag: "latest",
			Digest: "sha256:" + strings.Repeat("c", 64),
			Platform: executorregistration.Platform{OS: "linux", Architecture: "amd64"},
		},
		Observed: Observed{
			ContainerID: strings.Repeat("a", 64),
			ImageID: "sha256:" + strings.Repeat("b", 64),
		},
	}
}

func sampleBytes(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(sampleProposal())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseProposalFingerprintAndExactRegistrationMatch(t *testing.T) {
	data := sampleBytes(t)
	record, err := Parse(data, sampleNow)
	if err != nil {
		t.Fatalf("valid proposal rejected: %v", err)
	}
	sum := sha256.Sum256(data)
	if record.SHA256 != "sha256:"+hex.EncodeToString(sum[:]) || record.Size != len(data) {
		t.Fatalf("fingerprint must refer to exact input bytes: %#v", record)
	}
	uid := uint32(1001)
	reg := executorregistration.Registration{
		SchemaVersion: 1, RegistrationID: "hello-go-demo",
		ProjectName: "hello-go", ProjectDirectory: "/srv/deployments/hello-go",
		ComposeFiles: []string{"/srv/deployments/hello-go/docker-compose.yaml"},
		TrustedSourceUID: &uid,
		Services: []executorregistration.Service{{
			Name: "web", DeploymentEnabled: true,
			ApprovedRepository: "docker.io/example/hello-go",
			Platform: executorregistration.Platform{OS: "linux", Architecture: "amd64"},
		}},
		FeaturePolicy: executorregistration.FeaturePolicyPrebuilt,
	}
	if !record.Proposal.MatchesRegistration(reg) {
		t.Fatal("registered target should match (not deployment authorization)")
	}
	reg.Services[0].DeploymentEnabled = false
	if record.Proposal.MatchesRegistration(reg) {
		t.Fatal("monitor proposal reenabled disabled service")
	}
	reg.Services[0].DeploymentEnabled = true
	reg.Services[0].ApprovedRepository = "docker.io/example/unrelated"
	if record.Proposal.MatchesRegistration(reg) {
		t.Fatal("monitor proposal redirected approved repository")
	}
	reg.Services[0].ApprovedRepository = "docker.io/example/hello-go"
	reg.RegistrationID = "different-registration"
	if record.Proposal.MatchesRegistration(reg) {
		t.Fatal("monitor proposal forged registration identity")
	}
}

func TestUnsafeProposalFieldsFailClosed(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Proposal)
	}{
		{"wrong-schema", func(p *Proposal) { p.SchemaVersion = 2 }},
		{"bad-id-traversal", func(p *Proposal) { p.ProposalID = "../escape" }},
		{"bad-registration", func(p *Proposal) { p.RegistrationID = "/root/registrations" }},
		{"bad-service", func(p *Proposal) { p.Service = "web;docker" }},
		{"different-registry", func(p *Proposal) { p.Candidate.Repository = "evil.example/repo" }},
		{"tag-in-repository", func(p *Proposal) { p.Candidate.Repository += ":latest" }},
		{"uppercase-repository", func(p *Proposal) { p.Candidate.Repository = "docker.io/Example/hello-go" }},
		{"invalid-tag", func(p *Proposal) { p.Candidate.RequestedTag = "latest;down" }},
		{"tag-too-long", func(p *Proposal) { p.Candidate.RequestedTag = strings.Repeat("x", 129) }},
		{"invalid-digest", func(p *Proposal) { p.Candidate.Digest = "sha256:1234" }},
		{"uppercase-digest", func(p *Proposal) { p.Candidate.Digest = "sha256:" + strings.Repeat("A", 64) }},
		{"image-config-id-not-manifest", func(p *Proposal) { p.Candidate.Digest = "sha512:" + strings.Repeat("c", 64) }},
		{"wrong-platform", func(p *Proposal) { p.Candidate.Platform.Architecture = "arm64" }},
		{"missing-observed-container", func(p *Proposal) { p.Observed.ContainerID = "" }},
		{"short-container-id", func(p *Proposal) { p.Observed.ContainerID = "aabbccddeeff" }},
		{"bad-observed-image-id", func(p *Proposal) { p.Observed.ImageID = "/secrets" }},
		{"zero-lifetime", func(p *Proposal) { p.ExpiresAt = p.CreatedAt }},
		{"expired", func(p *Proposal) { p.ExpiresAt = sampleNow.Add(-time.Second).Format(time.RFC3339) }},
		{"too-long-lifetime", func(p *Proposal) { p.ExpiresAt = sampleNow.Add(16*time.Minute).Format(time.RFC3339) }},
		{"future-created", func(p *Proposal) {
			p.CreatedAt = sampleNow.Add(2*time.Minute).Format(time.RFC3339)
		}},
		{"non-utc-timestamp", func(p *Proposal) { p.CreatedAt = "2026-11-02T12:00:00-08:00" }},
		{"invalid-created-timestamp", func(p *Proposal) { p.CreatedAt = "tomorrow" }},
		{"expired-exactly-now", func(p *Proposal) { p.ExpiresAt = sampleNow.Format(time.RFC3339) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := sampleProposal()
			tc.edit(&p)
			data, err := json.Marshal(p)
			if err != nil { t.Fatal(err) }
			if _, err := Parse(data, sampleNow); !errors.Is(err, ErrInvalidProposal) {
				t.Fatalf("unsafe proposal accepted: %v", err)
			}
		})
	}
}

func TestStrictUntrustedJSONRejectsDuplicateUnknownAndTrailing(t *testing.T) {
	good := sampleBytes(t)
	cases := [][]byte{
		nil, []byte("{}"), []byte("null"), []byte("[]"),
		append(append([]byte{}, good...), []byte(" {}")...),
		bytes.Replace(good, []byte(`"proposal_id"`), []byte(`"proposal_id":"forged","proposal_id"`), 1),
		bytes.Replace(good, []byte(`"proposal_id"`), []byte(`"\u0070roposal_id":"forged","proposal_id"`), 1),
		bytes.Replace(good, []byte(`"repository"`), []byte(`"repository":"docker.io/evil/x","repository"`), 1),
		bytes.Replace(good, []byte(`"os"`), []byte(`"os":"linux","os"`), 1),
		bytes.Replace(good, []byte(`"image_id"`), []byte(`"docker_host":"tcp://evil:2375","image_id"`), 1),
		bytes.Replace(good, []byte(`"service"`), []byte(`"Service"`), 1),
		bytes.Replace(good, []byte(`"candidate":{`), []byte(`"candidate":null`), 1),
		bytes.Repeat([]byte("X"), MaxProposalBytes+1),
	}
	for i, b := range cases {
		if _, err := Parse(b, sampleNow); !errors.Is(err, ErrInvalidProposal) {
			t.Errorf("unsafe JSON case %d accepted: %v", i, err)
		}
	}
	if _, err := Parse(good, time.Time{}); !errors.Is(err, ErrInvalidProposal) {
		t.Fatal("zero trusted clock accepted")
	}
}

func TestCanonicalExpirationAndFingerprintChange(t *testing.T) {
	a := sampleBytes(t)
	p := sampleProposal()
	p.Candidate.RequestedTag = "beta1"
	b, err := json.Marshal(p)
	if err != nil { t.Fatal(err) }
	old, err := Parse(a, sampleNow)
	if err != nil { t.Fatal(err) }
	newRecord, err := Parse(b, sampleNow)
	if err != nil { t.Fatal(err) }
	if old.SHA256 == newRecord.SHA256 {
		t.Fatal("edited tag preserved fingerprint")
	}
	for _, input := range []string{"2026-11-02T19:59:00.000Z", "2026-11-02T19:59:00+00:00", "2026-11-02 19:59:00"} {
		p := sampleProposal()
		p.CreatedAt = input
		if err := p.Validate(sampleNow); !errors.Is(err, ErrInvalidProposal) {
			t.Errorf("noncanonical timestamp %q accepted: %v", input, err)
		}
	}
	// A forged ID may be syntactically valid but never give authority by itself.
	if !strings.HasPrefix(old.SHA256, "sha256:") || len(old.SHA256) != 71 {
		t.Fatal("fingerprint format invalid")
	}
}

func FuzzParseProposal(f *testing.F) {
	sample, _ := json.Marshal(sampleProposal())
	f.Add(sample)
	f.Add([]byte(`{"schema_version":1,"schema_version":2}`))
	f.Add([]byte(`{"candidate":{"digest":"sha256:abc","digest":"sha256:def"}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > MaxProposalBytes*2 { t.Skip() }
		record, err := Parse(data, sampleNow)
		if err == nil {
			if record.Proposal.Validate(sampleNow) != nil || record.Size != len(data) {
				t.Fatal(fmt.Errorf("parser accepted invalid proposal: %s", record.SHA256))
			}
		}
	})
}
