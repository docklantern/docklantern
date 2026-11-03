// Package executorpreview provides an informational, host-local preview of a
// bounded monitoring proposal and a protected registration. This is not
// deployment approval, a Docker inspection, or a substitute for B6-04.
package executorpreview

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/docklantern/docklantern/internal/executorproposal"
	"github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrPreviewDenied = errors.New("read-only host preview unavailable")

// Source implementations must independently read monitor-originated proposals
// and executor-private registrations. Monitoring state is never authority.
type Source interface {
	LoadProposal(string, time.Time) (executorproposal.Record, error)
	LoadRegistration(string) (executorregistration.Record, error)
}

// View records only syntactic eligibility and registration agreement.
// It deliberately contains no Docker actual baseline, Compose effective
// config, OCI verification, operator approval, or execution capability.
type View struct {
	ProposalID string
	RegistrationID string
	Project string
	Service string
	Repository string
	RequestedTag string
	CandidateDigest string
	Platform string
	ExpiresAt string
	ProposalSHA256 string
	RegistrationSHA256 string
	RunningOnlyAllowed bool
}

type snapshot struct {
	proposal executorproposal.Record
	registration executorregistration.Record
}

// Prepare reads each protected/untrusted source twice. A changed file or
// expired plan fails closed. The content hashes are evidence of byte
// identity, not signatures or authorization. No Docker state is inspected.
func Prepare(ctx context.Context, id string, source Source, now func() time.Time) (View, error) {
	var zero View
	if ctx == nil || source == nil || now == nil || !executorregistration.ValidID(id) || ctx.Err() != nil {
		return zero, ErrPreviewDenied
	}
	first, err := readSnapshot(ctx, id, source, now())
	if err != nil {
		return zero, ErrPreviewDenied
	}
	second, err := readSnapshot(ctx, id, source, now())
	if err != nil || first.proposal.SHA256 != second.proposal.SHA256 ||
		first.registration.SHA256 != second.registration.SHA256 {
		return zero, ErrPreviewDenied
	}
	plan, reg := second.proposal.Proposal, second.registration.Registration
	runningOnly := false
	for _, service := range reg.Services {
		if service.Name == plan.Service {
			runningOnly = service.AllowRunningOnly
			break
		}
	}
	return View{
		ProposalID: id, RegistrationID: reg.RegistrationID,
		Project: reg.ProjectName, Service: plan.Service,
		Repository: plan.Candidate.Repository, RequestedTag: plan.Candidate.RequestedTag,
		CandidateDigest: plan.Candidate.Digest,
		Platform: plan.Candidate.Platform.OS + "/" + plan.Candidate.Platform.Architecture,
		ExpiresAt: plan.ExpiresAt, ProposalSHA256: second.proposal.SHA256,
		RegistrationSHA256: second.registration.SHA256,
		RunningOnlyAllowed: runningOnly,
	}, nil
}

func readSnapshot(ctx context.Context, id string, src Source, now time.Time) (snapshot, error) {
	var empty snapshot
	if ctx.Err() != nil || now.IsZero() {
		return empty, ErrPreviewDenied
	}
	p, err := src.LoadProposal(id, now)
	if err != nil || !validHash(p.SHA256) || p.Proposal.ProposalID != id ||
		p.Proposal.Validate(now) != nil {
		return empty, ErrPreviewDenied
	}
	r, err := src.LoadRegistration(p.Proposal.RegistrationID)
	if err != nil || !validHash(r.SHA256) || r.Registration.Validate() != nil ||
		r.Registration.RegistrationID != p.Proposal.RegistrationID ||
		!p.Proposal.MatchesRegistration(r.Registration) {
		return empty, ErrPreviewDenied
	}
	return snapshot{proposal: p, registration: r}, nil
}

func validHash(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, ch := range value[7:] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

// Render is deliberately limited to parsed, bounded identity fields.
// No raw registration JSON, Compose source paths, environment, Docker output,
// credentials, or untrusted free-form error strings are shown.
func (v View) Render() string {
	readiness := "HEALTHCHECK required if a future deployment is approved"
	if v.RunningOnlyAllowed {
		readiness = "Running-only permitted by registration (weaker than healthy)"
	}
	return fmt.Sprintf(
		"DOCKLANTERN — LOCAL CANDIDATE PREVIEW (READ ONLY)\n"+
			"Proposal: %s\nRegistration: %s\nProject: %s\nService: %s\n"+
			"Repository: %s\nRequested tag (hint only): %s\n"+
			"Candidate digest (unverified): %s\nPlatform requested: %s\n"+
			"Expires (UTC): %s\nProposal SHA-256: %s\nRegistration SHA-256: %s\n"+
			"Readiness policy: %s\n"+
			"Actual Docker/container baseline: NOT INSPECTED\n"+
			"Effective Compose configuration: NOT INSPECTED\n"+
			"OCI digest and platform authenticity: NOT VERIFIED\n"+
			"DEPLOYMENT AUTHORIZATION: NOT GRANTED\n"+
			"No update, TTY deployment confirmation, or recovery will be performed.\n",
		v.ProposalID, v.RegistrationID, v.Project, v.Service, v.Repository,
		v.RequestedTag, v.CandidateDigest, v.Platform, v.ExpiresAt,
		v.ProposalSHA256, v.RegistrationSHA256, readiness,
	)
}
