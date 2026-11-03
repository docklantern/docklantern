package executorjournal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/docklantern/docklantern/internal/executorregistration"
)

const OverrideManifestVersion = 1

var ErrOverrideStage = errors.New("protected override staging unavailable")

// OverrideManifest is immutable, bounded metadata ONLY. It does not contain
// Compose YAML, paths, environment values or Docker/CLI arguments; a manifest
// cannot be supplied to Compose as an override. Actual image-only Compose
// semantics are gated by B6-04 and no committed override is changed here.
type OverrideManifest struct {
	SchemaVersion int    `json:"schema_version"`
	ProjectID string     `json:"project_id"`
	OperationID string   `json:"operation_id"`
	Generation int64     `json:"generation"`
	PreviousGeneration int64 `json:"previous_generation"`
	PreviousCommittedSHA256 string `json:"previous_committed_sha256"`
	CandidateImageDigest string `json:"candidate_image_digest"`
}

type OverrideGenerationView struct {
	ProjectID string
	OperationID string
	Generation int64
	ManifestSHA256 string
	CurrentStage string
	NeedsReconciliation bool
	Committed bool
}

func (m OverrideManifest) bytes() ([]byte, error) {
	if m.SchemaVersion != OverrideManifestVersion ||
		!executorregistration.ValidID(m.ProjectID) ||
		!executorregistration.ValidID(m.OperationID) ||
		m.Generation < 1 || m.Generation > 999999999999999999 ||
		m.PreviousGeneration != m.Generation-1 ||
		!validHash(m.PreviousCommittedSHA256) ||
		!validHash(m.CandidateImageDigest) {
		return nil, ErrOverrideStage
	}
	b, err := json.Marshal(m)
	if err != nil || len(b) > 1024 { return nil, ErrOverrideStage }
	return b,nil
}

func (s *Store) appendStageIntent(ctx context.Context, m OverrideManifest, hash string, at time.Time) error {
	if s == nil || ctx == nil || ctx.Err() != nil || !validAt(at) ||
		!validHash(hash) { return ErrOverrideStage }
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil || s.inFlight[m.OperationID] != "STAGE" {
		return ErrOverrideStage
	}
	tx, err := s.db.BeginTx(ctx,nil)
	if err != nil { return ErrOverrideStage }
	defer tx.Rollback()
	var project,kind,candidate,outcome string
	var updatedNS int64
	err=tx.QueryRowContext(ctx, `SELECT project_id,operation_kind,candidate_digest,outcome,updated_ns
		FROM operations WHERE operation_id=?`,m.OperationID).
		Scan(&project,&kind,&candidate,&outcome,&updatedNS)
	if err != nil || project != m.ProjectID || candidate != m.CandidateImageDigest ||
		outcome != OutcomePending || at.UnixNano() < updatedNS ||
		(kind != "apply" && kind != "recover") {
		return ErrOverrideStage
	}
	var previous int64
	var previousHash string
	err=tx.QueryRowContext(ctx,`SELECT generation,committed_sha256
		FROM override_records WHERE project_id=? ORDER BY generation DESC LIMIT 1`,m.ProjectID).
		Scan(&previous,&previousHash)
	// The initial committed generation must be separately provisioned and
	// verified: never infer a zero state from an absent database row.
	if err != nil || previous != m.PreviousGeneration ||
		previousHash != m.PreviousCommittedSHA256 { return ErrOverrideStage }
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM override_generation_events
		WHERE project_id=?`,m.ProjectID).Scan(&count); err != nil || count != 0 {
		// Phase 3c-a has no promotion completion API. Any earlier stage
		// leaves the project unresolved; never silently supersede it.
		return ErrOverrideStage
	}
	_,err=tx.ExecContext(ctx,`INSERT INTO override_generation_events
		(project_id,generation,operation_id,event_sequence,event_kind,
		 manifest_sha256,candidate_digest,previous_generation,
		 previous_committed_sha256,recorded_ns)
		VALUES (?,?,?,1,'stage_intent',?,?,?,?,?)`,
		m.ProjectID,m.Generation,m.OperationID,hash,m.CandidateImageDigest,
		m.PreviousGeneration,m.PreviousCommittedSHA256,at.UnixNano())
	if err != nil { return ErrOverrideStage }
	if err:=tx.Commit();err!=nil { return ErrOverrideStage }
	return nil
}

func (s *Store) appendStaged(ctx context.Context, m OverrideManifest, hash string, at time.Time) error {
	return s.appendFollowUp(ctx,m,hash,"staged",2,at)
}

func (s *Store) appendPromotionIntent(ctx context.Context, m OverrideManifest, hash string, at time.Time) error {
	return s.appendFollowUp(ctx,m,hash,"promotion_intent",3,at)
}

func (s *Store) appendFollowUp(ctx context.Context, m OverrideManifest, hash, kind string, seq int, at time.Time) error {
	if s == nil || ctx == nil || ctx.Err() != nil || !validAt(at) || !validHash(hash) {
		return ErrOverrideStage
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	needed := "STAGE"
	if kind == "promotion_intent" { needed = "COMMIT" }
	if s.db == nil || s.inFlight[m.OperationID] != needed { return ErrOverrideStage }
	tx,err:=s.db.BeginTx(ctx,nil)
	if err!=nil {return ErrOverrideStage}
	defer tx.Rollback()
	var project, candidate, outcome string
	var updatedNS int64
	err=tx.QueryRowContext(ctx,`SELECT project_id,candidate_digest,outcome,updated_ns
		FROM operations WHERE operation_id=?`,m.OperationID).
		Scan(&project,&candidate,&outcome,&updatedNS)
	if err!=nil || project!=m.ProjectID || candidate!=m.CandidateImageDigest ||
		outcome!=OutcomePending || at.UnixNano()<updatedNS { return ErrOverrideStage }
	var lastSeq int
	var lastKind,lastHash,lastCandidate,lastPreviousHash string
	var prevGeneration,lastNS int64
	err=tx.QueryRowContext(ctx,`SELECT event_sequence,event_kind,manifest_sha256,
		candidate_digest,previous_generation,previous_committed_sha256,recorded_ns
		FROM override_generation_events WHERE project_id=? AND generation=?
		ORDER BY event_sequence DESC LIMIT 1`,m.ProjectID,m.Generation).
		Scan(&lastSeq,&lastKind,&lastHash,&lastCandidate,
			&prevGeneration,&lastPreviousHash,&lastNS)
	expected:="stage_intent"
	if kind=="promotion_intent" {expected="staged"}
	if err!=nil || lastSeq!=seq-1 || lastKind!=expected ||
		lastHash!=hash || lastCandidate!=m.CandidateImageDigest ||
		prevGeneration!=m.PreviousGeneration ||
		lastPreviousHash!=m.PreviousCommittedSHA256 ||
		lastNS>at.UnixNano() {
		return ErrOverrideStage
	}
	_,err=tx.ExecContext(ctx,`INSERT INTO override_generation_events
		(project_id,generation,operation_id,event_sequence,event_kind,
		 manifest_sha256,candidate_digest,previous_generation,
		 previous_committed_sha256,recorded_ns)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,m.ProjectID,m.Generation,m.OperationID,seq,
		kind,hash,m.CandidateImageDigest,m.PreviousGeneration,
		m.PreviousCommittedSHA256,at.UnixNano())
	if err!=nil {return ErrOverrideStage}
	if err:=tx.Commit();err!=nil {return ErrOverrideStage}
	return nil
}

// InspectOverrideGeneration is read-only. The only states in this PR are
// intent, staged, promotion_intent: never promotion_completed or succeeded.
// No event can unblock deployment or change committed desired.
func (s *Store) InspectOverrideGeneration(ctx context.Context, projectID string) (OverrideGenerationView,error) {
	var empty OverrideGenerationView
	if s==nil || ctx==nil || ctx.Err()!=nil || !executorregistration.ValidID(projectID) {
		return empty,ErrOverrideStage
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db==nil {return empty,ErrOverrideStage}
	var view OverrideGenerationView
	err:=s.db.QueryRowContext(ctx,`SELECT project_id,operation_id,generation,
		manifest_sha256,event_kind FROM override_generation_events WHERE project_id=?
		ORDER BY generation DESC,event_sequence DESC LIMIT 1`,projectID).
		Scan(&view.ProjectID,&view.OperationID,&view.Generation,&view.ManifestSHA256,&view.CurrentStage)
	if errors.Is(err,sql.ErrNoRows){return empty,ErrOverrideStage}
	if err!=nil{return empty,ErrOverrideStage}
	view.NeedsReconciliation=true
	view.Committed=false
	return view,nil
}
