package executorjournal

import (
	"context"
	"database/sql"
	"errors"
)

const SchemaVersion = 1

var (
	ErrUnavailable = errors.New("protected executor journal unavailable")
	ErrUnsupportedSchema = errors.New("unsupported executor journal schema")
	ErrInvalidOperation = errors.New("invalid executor journal operation")
	ErrProjectBlocked = errors.New("project has unresolved executor operation")
	ErrInvalidTransition = errors.New("invalid executor journal phase transition")
)

// schemaV1 is intentionally independent of monitoring SQLite and contains no
// Docker commands, source Compose payloads, environment values, or raw logs.
// The append-only tables have database-enforced UPDATE/DELETE denial, not just
// application conventions. Their future writer APIs require separate review.
const schemaV1 = `
CREATE TABLE operations (
    operation_id TEXT PRIMARY KEY NOT NULL,
    operation_kind TEXT NOT NULL CHECK (operation_kind IN ('apply', 'recover')),
    project_id TEXT NOT NULL,
    registration_id TEXT NOT NULL,
    service_id TEXT NOT NULL,
    proposal_id TEXT NOT NULL,
    proposal_sha256 TEXT NOT NULL,
    candidate_digest TEXT NOT NULL,
    created_ns INTEGER NOT NULL CHECK (created_ns > 0),
    updated_ns INTEGER NOT NULL CHECK (updated_ns >= created_ns),
    current_phase TEXT NOT NULL DEFAULT 'NONE'
        CHECK (current_phase IN ('NONE', 'PREPARE', 'AUTHORIZE', 'PREFLIGHT',
            'PULL', 'STAGE', 'APPLY', 'VERIFY', 'COMMIT')),
    outcome TEXT NOT NULL DEFAULT 'pending'
        CHECK (outcome IN ('pending', 'needs_reconciliation', 'succeeded',
            'failed_no_change', 'recovered', 'failed_recovery'))
);
CREATE UNIQUE INDEX one_unresolved_operation_per_project
    ON operations(project_id)
    WHERE outcome IN ('pending', 'needs_reconciliation');
CREATE TRIGGER operations_immutable_identity BEFORE UPDATE OF
    operation_id, operation_kind, project_id, registration_id, service_id,
    proposal_id, proposal_sha256, candidate_digest, created_ns ON operations
    BEGIN SELECT RAISE(ABORT, 'immutable operation identity'); END;
CREATE TRIGGER operations_no_delete BEFORE DELETE ON operations
    BEGIN SELECT RAISE(ABORT, 'append-only operation history'); END;

CREATE TABLE phase_events (
    operation_id TEXT NOT NULL REFERENCES operations(operation_id),
    event_sequence INTEGER NOT NULL CHECK (event_sequence >= 1 AND event_sequence <= 18),
    phase TEXT NOT NULL CHECK (phase IN ('PREPARE', 'AUTHORIZE', 'PREFLIGHT',
        'PULL', 'STAGE', 'APPLY', 'VERIFY', 'COMMIT')),
    event_kind TEXT NOT NULL CHECK (event_kind IN ('intent', 'completed', 'uncertain')),
    event_ns INTEGER NOT NULL CHECK (event_ns > 0),
    message_code TEXT NOT NULL DEFAULT 'none'
        CHECK (message_code IN ('none', 'interrupted', 'state_ambiguous',
            'external_change', 'storage_uncertain')),
    PRIMARY KEY (operation_id, event_sequence)
);
CREATE TRIGGER phase_events_no_update BEFORE UPDATE ON phase_events
    BEGIN SELECT RAISE(ABORT, 'append-only phase history'); END;
CREATE TRIGGER phase_events_no_delete BEFORE DELETE ON phase_events
    BEGIN SELECT RAISE(ABORT, 'append-only phase history'); END;

CREATE TABLE baselines (
    operation_id TEXT PRIMARY KEY NOT NULL REFERENCES operations(operation_id),
    container_id TEXT NOT NULL,
    image_id TEXT NOT NULL,
    platform TEXT NOT NULL,
    source_fingerprint TEXT NOT NULL,
    desired_image_fingerprint TEXT NOT NULL,
    health_grade TEXT NOT NULL CHECK
       (health_grade IN ('unknown', 'healthy', 'running_only', 'unhealthy')),
    recorded_ns INTEGER NOT NULL CHECK (recorded_ns > 0)
);
CREATE TRIGGER baselines_no_update BEFORE UPDATE ON baselines
    BEGIN SELECT RAISE(ABORT, 'immutable recorded baseline'); END;
CREATE TRIGGER baselines_no_delete BEFORE DELETE ON baselines
    BEGIN SELECT RAISE(ABORT, 'immutable recorded baseline'); END;

CREATE TABLE override_records (
    project_id TEXT NOT NULL,
    generation INTEGER NOT NULL CHECK (generation >= 0),
    committed_sha256 TEXT NOT NULL,
    previous_sha256 TEXT,
    staged_operation_id TEXT REFERENCES operations(operation_id),
    promotion_intent_ns INTEGER,
    promotion_complete_ns INTEGER,
    PRIMARY KEY (project_id, generation)
);
CREATE TRIGGER override_records_no_update BEFORE UPDATE ON override_records
    BEGIN SELECT RAISE(ABORT, 'append-only override history'); END;
CREATE TRIGGER override_records_no_delete BEFORE DELETE ON override_records
    BEGIN SELECT RAISE(ABORT, 'append-only override history'); END;

CREATE TABLE receipt_publications (
    operation_id TEXT NOT NULL REFERENCES operations(operation_id),
    receipt_version INTEGER NOT NULL CHECK (receipt_version >= 1),
    content_sha256 TEXT NOT NULL,
    publication_state TEXT NOT NULL CHECK
       (publication_state IN ('pending', 'published', 'uncertain')),
    recorded_ns INTEGER NOT NULL CHECK (recorded_ns > 0),
    PRIMARY KEY (operation_id, receipt_version, publication_state)
);
CREATE TRIGGER receipt_publications_no_update BEFORE UPDATE ON receipt_publications
    BEGIN SELECT RAISE(ABORT, 'append-only receipt history'); END;
CREATE TRIGGER receipt_publications_no_delete BEFORE DELETE ON receipt_publications
    BEGIN SELECT RAISE(ABORT, 'append-only receipt history'); END;
`

// migrate is transactionally versioned; unknown newer DBs always fail closed.
// Version zero on a pre-populated unrelated database fails at CREATE TABLE
// rather than silently adopting or overwriting its schema.
func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return ErrUnavailable
	}
	switch version {
	case SchemaVersion:
		// Existing versions are never mutated by the migration path.
	case 0:
		if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
			return ErrUnavailable
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
			return ErrUnavailable
		}
	default:
		return ErrUnsupportedSchema
	}
	if err := tx.Commit(); err != nil {
		return ErrUnavailable
	}
	var quickCheck string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&quickCheck); err != nil || quickCheck != "ok" {
		return ErrUnavailable
	}
	// An unexpected/missing migration table must not masquerade as a
	// compatible user_version set by some unrelated SQLite application.
	for _, name := range []string{"operations", "phase_events", "baselines", "override_records", "receipt_publications"} {
		var kind string
		if err := db.QueryRowContext(ctx,
			"SELECT type FROM sqlite_master WHERE name = ?", name).Scan(&kind); err != nil || kind != "table" {
			return ErrUnsupportedSchema
		}
	}
	// A forged user_version must not bypass required append-only triggers.
	for _, trigger := range []string{
		"operations_immutable_identity", "operations_no_delete",
		"phase_events_no_update", "phase_events_no_delete",
		"baselines_no_update", "baselines_no_delete",
		"override_records_no_update", "override_records_no_delete",
		"receipt_publications_no_update", "receipt_publications_no_delete",
	} {
		var kind string
		if err := db.QueryRowContext(ctx,
			"SELECT type FROM sqlite_master WHERE name = ?", trigger).Scan(&kind); err != nil || kind != "trigger" {
			return ErrUnsupportedSchema
		}
	}
	return nil
}
