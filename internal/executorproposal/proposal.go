// Package executorproposal parses untrusted monitor-originated proposals.
// Proposals, fingerprints, and observed fields NEVER grant Docker authority.
package executorproposal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/docklantern/docklantern/internal/executorregistration"
)

const (
	SchemaVersion      = 1
	MaxProposalBytes   = 16 * 1024
	MaxProposalAge     = 15 * time.Minute
	MaxFutureClockSkew = time.Minute
	MaxPending         = 64
)

var ErrInvalidProposal = errors.New("invalid untrusted deployment proposal")

var (
	digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	idRE     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	tagRE    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
)

// Proposal is a monitoring hint, not an executable action or authorization.
// In particular, observed fields are NEVER a trusted Docker baseline.
type Proposal struct {
	SchemaVersion  int       `json:"schema_version"`
	ProposalID     string    `json:"proposal_id"`
	CreatedAt      string    `json:"created_at"`
	ExpiresAt      string    `json:"expires_at"`
	RegistrationID string    `json:"registration_id"`
	Service        string    `json:"service"`
	Candidate      Candidate `json:"candidate"`
	Observed       Observed  `json:"observed"`
}

type Candidate struct {
	Repository   string                        `json:"repository"`
	RequestedTag string                        `json:"requested_tag"`
	Digest       string                        `json:"digest"`
	Platform     executorregistration.Platform `json:"platform"`
}

type Observed struct {
	ContainerID string `json:"container_id"`
	ImageID     string `json:"image_id"`
}

// Record is a validated snapshot of EXACT bytes. A later, separately
// authorized preview/confirmation must reopen the proposal by opaque ID and
// compare this fingerprint before using any evidence. A hash is NOT approval.
type Record struct {
	Proposal Proposal
	SHA256   string
	Size     int
}

// Parse rejects malformed/unknown/duplicate-key JSON, stale or arbitrarily
// future proposals, platform/repository deviations and unsafe identity fields.
// It never reads Docker or the protected executor registration by itself.
func Parse(data []byte, now time.Time) (Record, error) {
	var empty Record
	if len(data) == 0 || len(data) > MaxProposalBytes || now.IsZero() {
		return empty, ErrInvalidProposal
	}
	if err := validateJSONSchema(data); err != nil {
		return empty, ErrInvalidProposal
	}
	var proposal Proposal
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return empty, ErrInvalidProposal
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return empty, ErrInvalidProposal
	}
	if err := proposal.Validate(now); err != nil {
		return empty, ErrInvalidProposal
	}
	sum := sha256.Sum256(data)
	return Record{Proposal: proposal, SHA256: "sha256:" + hex.EncodeToString(sum[:]), Size: len(data)}, nil
}

// Validate establishes only syntactic and expiration rules; it cannot prove
// a new image is safe, current, downloadable, or allowed to be deployed.
func (p Proposal) Validate(now time.Time) error {
	if p.SchemaVersion != SchemaVersion ||
		!executorregistration.ValidID(p.ProposalID) ||
		!executorregistration.ValidID(p.RegistrationID) ||
		!executorregistration.ValidID(p.Service) ||
		!executorregistration.ValidRepository(p.Candidate.Repository) ||
		!tagRE.MatchString(p.Candidate.RequestedTag) ||
		!digestRE.MatchString(p.Candidate.Digest) ||
		p.Candidate.Platform.OS != "linux" ||
		p.Candidate.Platform.Architecture != "amd64" ||
		!idRE.MatchString(p.Observed.ContainerID) ||
		!digestRE.MatchString(p.Observed.ImageID) || now.IsZero() {
		return ErrInvalidProposal
	}
	created, err := parseTimestamp(p.CreatedAt)
	if err != nil {
		return ErrInvalidProposal
	}
	expires, err := parseTimestamp(p.ExpiresAt)
	if err != nil {
		return ErrInvalidProposal
	}
	if !expires.After(created) || expires.Sub(created) > MaxProposalAge ||
		!expires.After(now) || created.After(now.Add(MaxFutureClockSkew)) {
		return ErrInvalidProposal
	}
	return nil
}

func parseTimestamp(s string) (time.Time, error) {
	if len(s) < 20 || len(s) > 40 || !strings.HasSuffix(s, "Z") {
		return time.Time{}, ErrInvalidProposal
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil || parsed.UTC().Format(time.RFC3339Nano) != s {
		return time.Time{}, ErrInvalidProposal
	}
	return parsed, nil
}

// MatchesRegistration verifies an exact *protected* allowlist match. Even a
// match grants NO authorization: B6-02c/B6-03/B6-04 must independently
// validate registration ownership, freshness, OCI evidence and actual Docker.
func (p Proposal) MatchesRegistration(reg executorregistration.Registration) bool {
	return p.RegistrationID == reg.RegistrationID &&
		reg.AllowsTarget(p.Service, p.Candidate.Repository, p.Candidate.Platform)
}

// validateJSONSchema decodes tokens rather than using map[string]any, which
// would silently accept duplicate object keys. It rejects unknown/case-
// variant keys at every depth, including escaped spellings of the same key.
func validateJSONSchema(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := readObject(dec, "proposal"); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return ErrInvalidProposal
	}
	return nil
}

func allowedKeys(object string) map[string]string {
	switch object {
	case "proposal":
		return map[string]string{
			"schema_version": "", "proposal_id": "", "created_at": "",
			"expires_at": "", "registration_id": "", "service": "",
			"candidate": "candidate", "observed": "observed",
		}
	case "candidate":
		return map[string]string{
			"repository": "", "requested_tag": "", "digest": "", "platform": "platform",
		}
	case "platform":
		return map[string]string{"os": "", "architecture": ""}
	case "observed":
		return map[string]string{"container_id": "", "image_id": ""}
	}
	return nil
}

func readObject(dec *json.Decoder, object string) error {
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return ErrInvalidProposal
	}
	allowed := allowedKeys(object)
	if allowed == nil {
		return ErrInvalidProposal
	}
	seen := make(map[string]bool, len(allowed))
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return ErrInvalidProposal
		}
		key, ok := keyToken.(string)
		if !ok || seen[key] {
			return ErrInvalidProposal
		}
		child, known := allowed[key]
		if !known {
			return ErrInvalidProposal
		}
		seen[key] = true
		if child != "" {
			if err := readObject(dec, child); err != nil {
				return err
			}
		} else {
			value, err := dec.Token()
			if err != nil || value == nil {
				return ErrInvalidProposal
			}
			switch value.(type) {
			case string, float64, json.Number:
			default:
				return ErrInvalidProposal
			}
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') {
		return ErrInvalidProposal
	}
	return nil
}
