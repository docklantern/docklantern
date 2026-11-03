// Package executorjournal implements a protected, standalone, versioned
// SQLite operation ledger. Phase 3a records intent and uncertainty ONLY:
// no Docker, Compose, override promotion, receipt publishing or success
// authorization is implemented in this package.
package executorjournal

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/docklantern/docklantern/internal/executorregistration"
	_ "modernc.org/sqlite"
)

const (
	OutcomePending = "pending"
	OutcomeNeedsReconciliation = "needs_reconciliation"
	EventIntent = "intent"
	EventCompleted = "completed"
	EventUncertain = "uncertain"
)

var phases = []string{"PREPARE", "AUTHORIZE", "PREFLIGHT", "PULL", "STAGE", "APPLY", "VERIFY", "COMMIT"}

// Store retains non-persistent phase ownership: a process that restarts
// cannot complete a side-effect phase started before that restart.
type Store struct {
	db *sql.DB
	mu sync.Mutex
	inFlight map[string]string
}

type Operation struct {
	ID string
	Kind string // apply or recover; operation metadata, NOT execution rights
	ProjectID string
	RegistrationID string
	ServiceID string
	ProposalID string
	ProposalSHA256 string
	CandidateDigest string
	CreatedAt time.Time
	UpdatedAt time.Time
	CurrentPhase string
	Outcome string
}

type PhaseEvent struct {
	Sequence int
	Phase string
	Kind string
	At time.Time
	Code string
}

type Snapshot struct {
	Operation Operation
	Events []PhaseEvent
	// EffectiveOutcome is conservative: an unmatched durable intent becomes
	// needs_reconciliation on read even when a crash prevented a state update.
	EffectiveOutcome string
	BlocksNewWork bool
}

// openSQLite is intentionally unexported and is used only for disposable
// tests and from the separately protected root host opener. Do not introduce
// a CLI flag or monitor-controlled path leading to this function.
func openSQLite(path string) (*Store, error) {
	if path == "" { return nil, ErrUnavailable }
	db, err := sql.Open("sqlite", path)
	if err != nil { return nil, ErrUnavailable }
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Single connection guarantees these connection-scoped PRAGMAs apply to
	// all transactions. WAL does NOT make Docker/override effects atomic.
	for _, pragma := range []string{
		"PRAGMA busy_timeout = 1000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = FULL",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, ErrUnavailable
		}
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db:db,inFlight:make(map[string]string)}, nil
}

func (s *Store) Close() error {
	if s == nil { return ErrUnavailable }
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil { return ErrUnavailable }
	db:=s.db
	s.db=nil
	s.inFlight=nil
	if err := db.Close(); err != nil { return ErrUnavailable }
	return nil
}

func validHash(hash string) bool {
	if len(hash) != 71 || !strings.HasPrefix(hash, "sha256:") { return false }
	for _, b := range hash[7:] {
		if (b < '0' || b > '9') && (b < 'a' || b > 'f') { return false }
	}
	return true
}

func validInput(op Operation) bool {
	return executorregistration.ValidID(op.ID) &&
		(op.Kind == "apply" || op.Kind == "recover") &&
		executorregistration.ValidID(op.ProjectID) &&
		executorregistration.ValidID(op.RegistrationID) &&
		executorregistration.ValidID(op.ServiceID) &&
		executorregistration.ValidID(op.ProposalID) &&
		validHash(op.ProposalSHA256) &&
		validHash(op.CandidateDigest) &&
		!op.CreatedAt.IsZero() && op.CreatedAt.UnixNano() > 0 &&
		op.UpdatedAt.IsZero() && op.CurrentPhase == "" && op.Outcome == ""
}

func validAt(at time.Time) bool {
	return !at.IsZero() && at.UnixNano() > 0
}

func phaseIndex(name string) int {
	for i, p := range phases { if name == p { return i } }
	return -1
}

func (s *Store) ensure() bool { return s != nil && s.db != nil }

// Begin records eligibility metadata only and blocks concurrent/unresolved
// operations for the same project via a DB-level unique partial index.
// This is NOT a host lock and it has no path to Docker or files.
func (s *Store) Begin(ctx context.Context, op Operation) error {
	if s == nil || ctx == nil || !validInput(op) { return ErrInvalidOperation }
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ensure() { return ErrUnavailable }
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return ErrUnavailable }
	defer tx.Rollback()
	var blocked int
	if err := tx.QueryRowContext(ctx, 
		"SELECT EXISTS(SELECT 1 FROM operations WHERE project_id = ? AND outcome IN ('pending','needs_reconciliation'))",
		op.ProjectID).Scan(&blocked); err != nil { return ErrUnavailable }
	if blocked != 0 { return ErrProjectBlocked }
	now := op.CreatedAt.UnixNano()
	_, err = tx.ExecContext(ctx, `INSERT INTO operations
		(operation_id,operation_kind,project_id,registration_id,service_id,proposal_id,
		 proposal_sha256,candidate_digest,created_ns,updated_ns,current_phase,outcome)
		VALUES (?,?,?,?,?,?,?,?,?,?,'NONE','pending')`,
		op.ID,op.Kind,op.ProjectID,op.RegistrationID,op.ServiceID,op.ProposalID,
		op.ProposalSHA256,op.CandidateDigest,now,now)
	if err != nil { return ErrUnavailable }
	if err := tx.Commit(); err != nil { return ErrUnavailable }
	return nil
}

type lastEvent struct {
	Seq int
	Phase string
	Kind string
	AtNS int64
}

func last(tx *sql.Tx, ctx context.Context, id string) (lastEvent, bool, error) {
	var e lastEvent
	err := tx.QueryRowContext(ctx, `SELECT event_sequence,phase,event_kind,event_ns
		FROM phase_events WHERE operation_id=? ORDER BY event_sequence DESC LIMIT 1`, id).
		Scan(&e.Seq,&e.Phase,&e.Kind,&e.AtNS)
	if errors.Is(err, sql.ErrNoRows) { return e,false,nil }
	return e,err==nil,err
}

func (s *Store) eventTx(ctx context.Context, id, phase, kind, code string, at time.Time) error {
	if !s.ensure() || ctx == nil || !executorregistration.ValidID(id) ||
		phaseIndex(phase)<0 || !validAt(at) {
		return ErrInvalidTransition
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return ErrUnavailable }
	defer tx.Rollback()
	var state string
	var recordedNS int64
	if err := tx.QueryRowContext(ctx,
		"SELECT outcome,updated_ns FROM operations WHERE operation_id=?",id).
		Scan(&state,&recordedNS); err != nil { return ErrInvalidTransition }
	if state != OutcomePending || at.UnixNano() < recordedNS { return ErrInvalidTransition }
	previous, found, err := last(tx,ctx,id)
	if err != nil { return ErrUnavailable }
	if found && at.UnixNano() < previous.AtNS { return ErrInvalidTransition }
	next := 1
	if found { next = previous.Seq+1 }
	if next > 2*len(phases)+1 { return ErrInvalidTransition }
	switch kind {
	case EventIntent:
		if code != "none" { return ErrInvalidTransition }
		if !found && phase != phases[0] { return ErrInvalidTransition }
		if found && (previous.Kind != EventCompleted ||
			phaseIndex(previous.Phase)+1 >= len(phases) ||
			phase != phases[phaseIndex(previous.Phase)+1]) { return ErrInvalidTransition }
	case EventCompleted:
		if code != "none" || !found || previous.Kind != EventIntent ||
			previous.Phase != phase { return ErrInvalidTransition }
	case EventUncertain:
		if code != "interrupted" && code != "state_ambiguous" &&
			code != "external_change" && code != "storage_uncertain" {
			return ErrInvalidTransition
		}
		if found && previous.Phase != phase { return ErrInvalidTransition }
		if !found && phase != "PREPARE" { return ErrInvalidTransition }
	default:
		return ErrInvalidTransition
	}
	_, err = tx.ExecContext(ctx,
		"INSERT INTO phase_events (operation_id,event_sequence,phase,event_kind,event_ns,message_code) VALUES (?,?,?,?,?,?)",
		id,next,phase,kind,at.UnixNano(),code)
	if err != nil { return ErrUnavailable }
	status := OutcomePending
	if kind == EventUncertain { status = OutcomeNeedsReconciliation }
	_, err = tx.ExecContext(ctx,
		"UPDATE operations SET current_phase=?,updated_ns=?,outcome=? WHERE operation_id=? AND outcome='pending'",
		phase,at.UnixNano(),status,id)
	if err != nil { return ErrUnavailable }
	if err := tx.Commit(); err != nil { return ErrUnavailable }
	return nil
}

// Intent MUST be committed before an independently approved side effect.
// Phase 3a offers NO effect-runner and no ability to finalize as succeeded.
func (s *Store) Intent(ctx context.Context, id, phase string, at time.Time) error {
	if s == nil { return ErrUnavailable }
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil { return ErrUnavailable }
	if _,exists:=s.inFlight[id];exists { return ErrInvalidTransition }
	if err:=s.eventTx(ctx,id,phase,EventIntent,"none",at);err!=nil {return err}
	s.inFlight[id]=phase
	return nil
}

// Complete is a journal fact about a phase, NOT proof of Docker success.
// No method in Phase 3a can set an operation to "succeeded"/"recovered".
func (s *Store) Complete(ctx context.Context, id, phase string, at time.Time) error {
	if s == nil { return ErrUnavailable }
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil || s.inFlight[id]!=phase { return ErrInvalidTransition }
	if err:=s.eventTx(ctx,id,phase,EventCompleted,"none",at);err!=nil {return err}
	delete(s.inFlight,id)
	return nil
}

func (s *Store) MarkUncertain(ctx context.Context, id, phase, code string, at time.Time) error {
	if s == nil { return ErrUnavailable }
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil { return ErrUnavailable }
	if err:=s.eventTx(ctx,id,phase,EventUncertain,code,at);err!=nil {return err}
	delete(s.inFlight,id)
	return nil
}

func (s *Store) Inspect(ctx context.Context, id string) (Snapshot, error) {
	var empty Snapshot
	if s == nil || ctx == nil || !executorregistration.ValidID(id) {
		return empty, ErrInvalidOperation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ensure() {
		return empty, ErrInvalidOperation
	}
	var op Operation
	var createdNS, updatedNS int64
	err := s.db.QueryRowContext(ctx, `SELECT operation_id,operation_kind,project_id,
		registration_id,service_id,proposal_id,proposal_sha256,candidate_digest,
		created_ns,updated_ns,current_phase,outcome FROM operations WHERE operation_id=?`,id).
		Scan(&op.ID,&op.Kind,&op.ProjectID,&op.RegistrationID,&op.ServiceID,&op.ProposalID,
			&op.ProposalSHA256,&op.CandidateDigest,&createdNS,&updatedNS,&op.CurrentPhase,&op.Outcome)
	if err != nil { return empty, ErrUnavailable }
	op.CreatedAt,op.UpdatedAt=time.Unix(0,createdNS).UTC(),time.Unix(0,updatedNS).UTC()
	rows, err := s.db.QueryContext(ctx, `SELECT event_sequence,phase,event_kind,event_ns,message_code
		FROM phase_events WHERE operation_id=? ORDER BY event_sequence`,id)
	if err != nil { return empty, ErrUnavailable }
	defer rows.Close()
	events:=make([]PhaseEvent,0,2*len(phases)+1)
	for rows.Next() {
		if len(events)>=2*len(phases)+1 { return empty, ErrUnavailable }
		var e PhaseEvent
		var ns int64
		if err := rows.Scan(&e.Sequence,&e.Phase,&e.Kind,&ns,&e.Code);err!=nil {return empty,ErrUnavailable}
		e.At=time.Unix(0,ns).UTC()
		events=append(events,e)
	}
	if rows.Err()!=nil { return empty,ErrUnavailable }
	effective:=op.Outcome
	if len(events)>0 && (events[len(events)-1].Kind==EventIntent ||
		events[len(events)-1].Kind==EventUncertain) {
		effective=OutcomeNeedsReconciliation
	}
	return Snapshot{Operation:op,Events:events,EffectiveOutcome:effective,
		BlocksNewWork:op.Outcome==OutcomePending || op.Outcome==OutcomeNeedsReconciliation},nil
}
