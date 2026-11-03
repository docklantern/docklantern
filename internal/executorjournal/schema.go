package executorjournal

import (
	"context"
	"database/sql"
	"errors"
)

const SchemaVersion = 2

// SQLite application_id identifies this file as executor-owned state.
const journalApplicationID = 1145851988

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

// schemaV2 records immutable generation staging and promotion intent
// independently of v1 committed override_records. A pending intent is NEVER
// interpreted as committed desired state. The future promotion completion
// must verify filesystem + actual state before inserting any committed record.
const schemaV2 = `
CREATE TABLE override_generation_events (
    project_id TEXT NOT NULL,
    generation INTEGER NOT NULL CHECK (generation >= 1),
    operation_id TEXT NOT NULL REFERENCES operations(operation_id),
    event_sequence INTEGER NOT NULL CHECK (event_sequence BETWEEN 1 AND 3),
    event_kind TEXT NOT NULL CHECK (event_kind IN
        ('stage_intent', 'staged', 'promotion_intent')),
    manifest_sha256 TEXT NOT NULL,
    candidate_digest TEXT NOT NULL,
    previous_generation INTEGER NOT NULL CHECK (previous_generation >= 0),
    previous_committed_sha256 TEXT NOT NULL,
    recorded_ns INTEGER NOT NULL CHECK (recorded_ns > 0),
    PRIMARY KEY (project_id, generation, event_sequence),
    UNIQUE (operation_id, event_sequence)
);
CREATE INDEX override_generation_events_by_project
    ON override_generation_events(project_id, generation, event_sequence);
CREATE TRIGGER override_generation_events_no_update BEFORE UPDATE ON override_generation_events
    BEGIN SELECT RAISE(ABORT, 'append-only generation history'); END;
CREATE TRIGGER override_generation_events_no_delete BEFORE DELETE ON override_generation_events
    BEGIN SELECT RAISE(ABORT, 'append-only generation history'); END;
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
	var appID int
	if err := tx.QueryRowContext(ctx,"PRAGMA application_id").Scan(&appID);err!=nil {
		return ErrUnavailable
	}
	switch version {
	case SchemaVersion:
		if appID != journalApplicationID { return ErrUnsupportedSchema }
	case 1:
		if appID != journalApplicationID { return ErrUnsupportedSchema }
		// Fail rather than adopting a damaged v1 schema before upgrading.
		if !requiredSchemaObjects(ctx, tx, false) { return ErrUnsupportedSchema }
		if _, err := tx.ExecContext(ctx, schemaV2); err != nil { return ErrUnavailable }
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 2"); err != nil {
			return ErrUnavailable
		}
	case 0:
		if appID != 0 { return ErrUnsupportedSchema }
		var otherObjects int
		if err := tx.QueryRowContext(ctx,`SELECT count(*) FROM sqlite_master
			WHERE name NOT LIKE 'sqlite_%'
			AND type IN ('table','view','index','trigger')`).Scan(&otherObjects); err != nil {
			return ErrUnavailable
		}
		if otherObjects != 0 { return ErrUnsupportedSchema }
		if _, err := tx.ExecContext(ctx, schemaV1); err != nil { return ErrUnavailable }
		if _, err := tx.ExecContext(ctx, schemaV2); err != nil { return ErrUnavailable }
		if _, err := tx.ExecContext(ctx, "PRAGMA application_id = 1145851988"); err != nil {
			return ErrUnavailable
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 2"); err != nil {
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
	if !requiredSchemaObjects(ctx, db, true) { return ErrUnsupportedSchema }
	return nil
}

// schemaQuerier supports live transactional upgrade preflight and final
// consistency validation.
type schemaQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requiredSchemaObjects(ctx context.Context, q schemaQuerier, v2 bool) bool {
	tables:=[]string{"operations","phase_events","baselines","override_records","receipt_publications"}
	triggers:=[]string{
		"operations_immutable_identity","operations_no_delete",
		"phase_events_no_update","phase_events_no_delete",
		"baselines_no_update","baselines_no_delete",
		"override_records_no_update","override_records_no_delete",
		"receipt_publications_no_update","receipt_publications_no_delete",
	}
	if v2 {
		tables=append(tables,"override_generation_events")
		triggers=append(triggers,"override_generation_events_no_update","override_generation_events_no_delete")
	}
	for _,name:=range tables{
		var kind string
		if err:=q.QueryRowContext(ctx,"SELECT type FROM sqlite_master WHERE name=?",name).Scan(&kind);err!=nil||kind!="table"{return false}
	}
	for _,name:=range triggers{
		var kind string
		if err:=q.QueryRowContext(ctx,"SELECT type FROM sqlite_master WHERE name=?",name).Scan(&kind);err!=nil||kind!="trigger"{return false}
	}
	return true
}
