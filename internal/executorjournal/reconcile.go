package executorjournal

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"

	"github.com/docklantern/docklantern/internal/executorregistration"
)

// ActualReader is intentionally only an interface. Phase 3b has NO Docker,
// Compose or OCI implementation: CI tests inject simulated observations.
// A real implementation requires separately approved B6-04 qualification.
type ActualReader interface {
	InspectActual(ctx context.Context, projectID, serviceID string) (ActualEvidence, error)
}

// ActualEvidence is bounded structured observation data, never commands.
type ActualEvidence struct {
	ProjectID string
	ServiceID string
	ContainerID string
	ImageID string
	Platform string
	SourceSHA256 string
	DesiredImageSHA256 string
	HealthGrade string
	OverrideGeneration int64
	CommittedOverrideSHA256 string
	ObservedAt time.Time
}

const (
	CheckNeedsReconciliation = "needs_reconciliation"
	CheckObservationsMatch = "observations_match_unqualified"
	MaxActualEvidenceAge = 10 * time.Second
)

// ReconciliationReport is informational only. It offers no success or
// approval claim, journal finalize, unlock, recovery or mutation capability.
type ReconciliationReport struct {
	OperationID string
	ProjectID string
	Status string
	ReasonCode string
	BlocksNewWork bool
}

type recordedBaseline struct {
	ContainerID string
	ImageID string
	Platform string
	SourceSHA256 string
	DesiredImageSHA256 string
	HealthGrade string
	RecordedNS int64
}

type recordedOverride struct {
	Generation int64
	CommittedSHA256 string
	PreviousSHA256 sql.NullString
	StagedOperationID sql.NullString
	PromotionIntentNS sql.NullInt64
	PromotionCompleteNS sql.NullInt64
}
type ledgerView struct {
	Snapshot Snapshot
	Baseline recordedBaseline
	HasBaseline bool
	Override recordedOverride
	HasOverride bool
	AmbiguousPromotions int
	UncommittedGenerationEvents int
}

// readLedger obtains ONE read-only SQLite transaction snapshot over the
// operation, all phase events, recorded baseline and override history.
func (s *Store) readLedger(ctx context.Context, id string) (ledgerView, error) {
	var empty ledgerView
	if s == nil || ctx == nil || ctx.Err() != nil || !executorregistration.ValidID(id) {
		return empty, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil { return empty, ErrUnavailable }
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil { return empty, ErrUnavailable }
	defer tx.Rollback()
	snapshot, err := inspectJournalTx(ctx, tx, id)
	if err != nil { return empty, ErrUnavailable }
	result := ledgerView{Snapshot: snapshot}
	err = tx.QueryRowContext(ctx, `SELECT container_id,image_id,platform,source_fingerprint,
		desired_image_fingerprint,health_grade,recorded_ns FROM baselines
		WHERE operation_id=?`, id).Scan(
		&result.Baseline.ContainerID, &result.Baseline.ImageID,
		&result.Baseline.Platform, &result.Baseline.SourceSHA256,
		&result.Baseline.DesiredImageSHA256, &result.Baseline.HealthGrade,
		&result.Baseline.RecordedNS)
	switch {
	case err == nil: result.HasBaseline = true
	case errors.Is(err, sql.ErrNoRows):
	default: return empty, ErrUnavailable
	}
	err = tx.QueryRowContext(ctx, `SELECT generation,committed_sha256,
		previous_sha256,staged_operation_id,promotion_intent_ns,promotion_complete_ns
		FROM override_records WHERE project_id=?
		ORDER BY generation DESC LIMIT 1`, snapshot.Operation.ProjectID).Scan(
		&result.Override.Generation, &result.Override.CommittedSHA256,
		&result.Override.PreviousSHA256, &result.Override.StagedOperationID,
		&result.Override.PromotionIntentNS, &result.Override.PromotionCompleteNS)
	switch {
	case err == nil: result.HasOverride = true
	case errors.Is(err, sql.ErrNoRows):
	default: return empty, ErrUnavailable
	}
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM override_records
		WHERE project_id=? AND
		((promotion_intent_ns IS NULL) != (promotion_complete_ns IS NULL)
		 OR (promotion_intent_ns IS NOT NULL
		 AND promotion_complete_ns < promotion_intent_ns))`,
		snapshot.Operation.ProjectID).Scan(&result.AmbiguousPromotions)
	if err != nil { return empty, ErrUnavailable }
	// Any Phase 3c stage/promotion intent is protected metadata, NOT a
	// committed override. This slice cannot certify promotion completion;
	// both an old restored DB and a half-staged generation must fail closed.
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM override_generation_events
		WHERE project_id=?`, snapshot.Operation.ProjectID).Scan(&result.UncommittedGenerationEvents)
	if err != nil || ctx.Err() != nil { return empty, ErrUnavailable }
	return result, nil
}

func validFullContainerID(s string) bool {
	if len(s) != 64 { return false }
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') { return false }
	}
	return true
}

func validGrade(s string) bool {
	return s == "healthy" || s == "running_only" || s == "unhealthy"
}

func uncertain(op Operation, code string) ReconciliationReport {
	return ReconciliationReport{
		OperationID: op.ID, ProjectID: op.ProjectID,
		Status: CheckNeedsReconciliation, ReasonCode: code, BlocksNewWork: true,
	}
}

// ReconcileReadOnly compares durable journal claims with independently
// supplied (currently FAKE-ONLY) Docker/Compose/override evidence. It is
// unable to mark an operation successful, mutate journal state, clear a
// project lock, promote files, or invoke Docker.
//
// Even an exact match is unqualified and never unblocks a project. Missing,
// stale, ambiguous, divergent, interrupted or changing evidence fails closed.
func (s *Store) ReconcileReadOnly(ctx context.Context, id string, src ActualReader, now time.Time) (ReconciliationReport, error) {
	if ctx == nil || src == nil || !validAt(now) || ctx.Err() != nil {
		return ReconciliationReport{}, ErrUnavailable
	}
	first, err := s.readLedger(ctx, id)
	if err != nil { return ReconciliationReport{}, ErrUnavailable }
	op := first.Snapshot.Operation
	if first.Snapshot.EffectiveOutcome == OutcomeNeedsReconciliation {
		return uncertain(op, "journal_unresolved"), nil
	}
	if first.UncommittedGenerationEvents != 0 {
		return uncertain(op,"override_generation_incomplete"),nil
	}
	if !first.HasBaseline { return uncertain(op, "baseline_missing"), nil }
	if !first.HasOverride { return uncertain(op, "override_missing"), nil }
	if first.AmbiguousPromotions != 0 ||
		first.Override.PromotionIntentNS.Valid != first.Override.PromotionCompleteNS.Valid {
		return uncertain(op, "override_promotion_ambiguous"), nil
	}
	base, override := first.Baseline, first.Override
	if !validFullContainerID(base.ContainerID) || !validHash(base.ImageID) ||
		base.Platform != "linux/amd64" || !validHash(base.SourceSHA256) ||
		!validHash(base.DesiredImageSHA256) || !validGrade(base.HealthGrade) ||
		base.RecordedNS <= 0 || base.RecordedNS < op.CreatedAt.UnixNano() ||
		base.RecordedNS > now.UnixNano() || !validHash(override.CommittedSHA256) ||
		override.Generation < 0 || (override.PreviousSHA256.Valid &&
			!validHash(override.PreviousSHA256.String)) ||
		(override.PromotionIntentNS.Valid && (override.PromotionIntentNS.Int64 <= 0 ||
			override.PromotionCompleteNS.Int64 > now.UnixNano())) {
		return uncertain(op, "invalid_recorded_state"), nil
	}
	inspectCtx, cancel := context.WithTimeout(ctx, MaxActualEvidenceAge)
	defer cancel()
	actual, err := src.InspectActual(inspectCtx, op.ProjectID, op.ServiceID)
	if err != nil || inspectCtx.Err() != nil {
		return uncertain(op, "backend_unavailable"), nil
	}
	if actual.ObservedAt.IsZero() || actual.ObservedAt.After(now) ||
		now.Sub(actual.ObservedAt) > MaxActualEvidenceAge {
		return uncertain(op, "backend_stale"), nil
	}
	// Re-read the ledger AFTER backend inspection; the function cannot be
	// mistaken for a stable, qualified execution preflight even if this
	// second snapshot matches.
	second, err := s.readLedger(ctx, id)
	if err != nil || !reflect.DeepEqual(first, second) || ctx.Err() != nil {
		return uncertain(op, "journal_changed"), nil
	}
	if actual.ProjectID != op.ProjectID || actual.ServiceID != op.ServiceID ||
		!validFullContainerID(actual.ContainerID) || !validHash(actual.ImageID) ||
		actual.Platform != base.Platform || !validHash(actual.SourceSHA256) ||
		!validHash(actual.DesiredImageSHA256) || !validGrade(actual.HealthGrade) ||
		actual.ContainerID != base.ContainerID || actual.ImageID != base.ImageID ||
		actual.SourceSHA256 != base.SourceSHA256 ||
		actual.DesiredImageSHA256 != base.DesiredImageSHA256 ||
		actual.HealthGrade != base.HealthGrade {
		return uncertain(op, "baseline_mismatch"), nil
	}
	if actual.OverrideGeneration != override.Generation ||
		!validHash(actual.CommittedOverrideSHA256) ||
		actual.CommittedOverrideSHA256 != override.CommittedSHA256 {
		return uncertain(op, "override_mismatch"), nil
	}
	return ReconciliationReport{
		OperationID: op.ID, ProjectID: op.ProjectID,
		Status: CheckObservationsMatch, ReasonCode: "unqualified_match",
		BlocksNewWork: true,
	}, nil
}
