// Package executorreview provides the Phase 2c *review-only* terminal gate.
// It cannot authorize an update: Phase 4 has not qualified fresh Docker and
// Compose evidence, and the standalone executor still has no write commands.
package executorreview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/docklantern/docklantern/internal/executorproposal"
	"github.com/docklantern/docklantern/internal/executorregistration"
)

var ErrReviewDenied = errors.New("host-local review denied")

const (
	MaxBaselineAge = 10 * time.Second
	MaxConfirmationDuration = 2 * time.Minute
)

// Baseline must be supplied by a separately trusted, fresh read-only Docker
// and stable Compose inspector. No such production inspector is wired in
// Phase 2c; the CI-only tests provide an explicitly simulated inspector.
type Baseline struct {
	ContainerID      string
	ImageID          string
	SourceFingerprint string
	CheckedAt        time.Time
}

// Source separates the protected-registration, untrusted proposal and
// independently inspected actual-state sources. Monitor observations are
// never accepted in place of InspectActual.
type Source interface {
	LoadProposal(id string, now time.Time) (executorproposal.Record, error)
	LoadRegistration(id string) (executorregistration.Record, error)
	InspectActual(context.Context, executorregistration.Record, string) (Baseline, error)
}

// Terminal must be a local controlling TTY, not stdin, HTTP or a web session.
// The production adapter only opens /dev/tty and insists on root EUID.
type Terminal interface {
	Interactive() bool
	Print(string) error
	ReadLine(context.Context) (string, error)
}

// Clock is injectable for deterministic expiry and clock-rollover tests.
type Clock func() time.Time

// Receipt is a non-transferable audit fact for a human review; this type has
// NO "Approved"/"Authorized"/capability flag and must never enable Docker.
type Receipt struct {
	RegistrationID string
	ProposalID     string
	ProposalSHA256 string
	RegistrationSHA256 string
	CandidateDigest string
	BaselineSHA256 string
	ReviewedAt time.Time
	ReviewOnly bool
}

// Review performs an independent local, interactive human acknowledgment.
// It rejects the whole interaction if the proposal, protected registration,
// fresh actual baseline, or expiry changes while waiting for input. The
// result is informational only; it is not an APPLY or RECOVER authorization.
func Review(ctx context.Context, id string, source Source, tty Terminal, isAdmin func() bool, now Clock) (Receipt, error) {
	var empty Receipt
	if ctx == nil || source == nil || tty == nil || isAdmin == nil || now == nil ||
		!isAdmin() || !tty.Interactive() || !executorregistration.ValidID(id) {
		return empty, ErrReviewDenied
	}
	if err := ctx.Err(); err != nil { return empty, ErrReviewDenied }
	first, err := snapshot(ctx, id, source, now())
	if err != nil { return empty, ErrReviewDenied }

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil { return empty, ErrReviewDenied }
	challenge := confirmationCode(first, nonce)
	note := renderReview(first, challenge)
	if err := tty.Print(note); err != nil { return empty, ErrReviewDenied }

	// The local TTY prompt is bounded even if a caller supplied a broader
	// deadline. Adapter implementations must unblock/cancel on context end.
	confirmCtx, cancel := context.WithTimeout(ctx, MaxConfirmationDuration)
	defer cancel()
	input, err := tty.ReadLine(confirmCtx)
	if err != nil || confirmCtx.Err() != nil || input != "REVIEW "+challenge {
		return empty, ErrReviewDenied
	}
	// This intentionally reopens both immutable registration authority and
	// the replaceable monitor proposal; compare exact-byte content hashes and
	// independently refreshed state AFTER the human input.
	second, err := snapshot(ctx, id, source, now())
	if err != nil || second.plan.SHA256 != first.plan.SHA256 ||
		second.registration.SHA256 != first.registration.SHA256 ||
		second.baselineHash != first.baselineHash {
		return empty, ErrReviewDenied
	}
	confirmedAt := now()
	if second.plan.Proposal.Validate(confirmedAt) != nil { return empty, ErrReviewDenied }
	return Receipt{
		RegistrationID: first.plan.Proposal.RegistrationID,
		ProposalID: id, ProposalSHA256: first.plan.SHA256,
		RegistrationSHA256: first.registration.SHA256,
		CandidateDigest: first.plan.Proposal.Candidate.Digest,
		BaselineSHA256: first.baselineHash,
		ReviewedAt: confirmedAt, ReviewOnly: true,
	}, nil
}

type reviewSnapshot struct {
	plan executorproposal.Record
	registration executorregistration.Record
	actual Baseline
	baselineHash string
}

func snapshot(ctx context.Context, id string, source Source, now time.Time) (reviewSnapshot, error) {
	var zero reviewSnapshot
	if err := ctx.Err(); err != nil || now.IsZero() { return zero, ErrReviewDenied }
	proposal, err := source.LoadProposal(id, now)
	if err != nil || proposal.Proposal.ProposalID != id || proposal.Proposal.Validate(now) != nil ||
		!validHash(proposal.SHA256) {
		return zero, ErrReviewDenied
	}
	registration, err := source.LoadRegistration(proposal.Proposal.RegistrationID)
	if err != nil || !validHash(registration.SHA256) ||
		registration.Registration.Validate() != nil ||
		registration.Registration.RegistrationID != proposal.Proposal.RegistrationID ||
		!proposal.Proposal.MatchesRegistration(registration.Registration) {
		return zero, ErrReviewDenied
	}
	actual, err := source.InspectActual(ctx, registration, proposal.Proposal.Service)
	if err != nil || !validHash(actual.SourceFingerprint) ||
		actual.ContainerID != proposal.Proposal.Observed.ContainerID ||
		actual.ImageID != proposal.Proposal.Observed.ImageID ||
		actual.CheckedAt.IsZero() || actual.CheckedAt.After(now) ||
		now.Sub(actual.CheckedAt) > MaxBaselineAge {
		return zero, ErrReviewDenied
	}
	h := sha256.Sum256([]byte(actual.ContainerID + "|" + actual.ImageID + "|" + actual.SourceFingerprint))
	return reviewSnapshot{proposal, registration, actual, "sha256:" + hex.EncodeToString(h[:])}, nil
}

func validHash(s string) bool {
	if len(s) != len("sha256:")+64 || !strings.HasPrefix(s, "sha256:") { return false }
	for _, c := range s[len("sha256:"):] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') { return false }
	}
	return true
}

func confirmationCode(s reviewSnapshot, nonce []byte) string {
	// JSON encoding prevents ambiguous delimiter concatenation. The nonce
	// ensures the operator cannot reuse an old response from a past session.
	payload, _ := json.Marshal(struct {
		Registration string
		Project string
		Service string
		Repository string
		Platform executorregistration.Platform
		Digest string
		Expires string
		ProposalHash string
		RegistrationHash string
		BaselineHash string
		Nonce string
	}{
		s.plan.Proposal.RegistrationID,
		s.registration.Registration.ProjectName,
		s.plan.Proposal.Service,
		s.plan.Proposal.Candidate.Repository,
		s.plan.Proposal.Candidate.Platform,
		s.plan.Proposal.Candidate.Digest,
		s.plan.Proposal.ExpiresAt,
		s.plan.SHA256,
		s.registration.SHA256,
		s.baselineHash,
		hex.EncodeToString(nonce),
	})
	sum := sha256.Sum256(payload)
	return strings.ToUpper(hex.EncodeToString(sum[:6]))
}

func renderReview(s reviewSnapshot, challenge string) string {
	p, r := s.plan.Proposal, s.registration.Registration
	var readiness string
	for _, service := range r.Services {
		if service.Name == p.Service && service.AllowRunningOnly {
			readiness = "Running-only verification permitted (weaker than HEALTHY)"
			break
		}
	}
	if readiness == "" { readiness = "HEALTHCHECK required" }
	return fmt.Sprintf(
		"DOCKLANTERN — READ-ONLY LOCAL REVIEW (NOT DEPLOYMENT APPROVAL)\n"+
		"Registration: %s\nProject: %s\nService: %s\n"+
		"Candidate repository: %s\nCandidate digest: %s\nPlatform: %s/%s\n"+
		"Expires (UTC): %s\nProposal SHA-256: %s\nRegistration SHA-256: %s\n"+
		"Actual baseline SHA-256: %s\nReadiness policy: %s\n"+
		"Type REVIEW %s to acknowledge exactly this read-only snapshot.\n"+
		"NO DOCKER MUTATION WILL OCCUR; DEPLOYMENT IS NOT QUALIFIED.\n",
		p.RegistrationID, r.ProjectName, p.Service,
		p.Candidate.Repository, p.Candidate.Digest,
		p.Candidate.Platform.OS, p.Candidate.Platform.Architecture,
		p.ExpiresAt, s.plan.SHA256, s.registration.SHA256,
		s.baselineHash, readiness, challenge,
	)
}

