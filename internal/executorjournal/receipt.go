package executorjournal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/docklantern/docklantern/internal/executorregistration"
)

const (
	ReceiptSchemaVersion = 1
	MaxReceiptBytes = 8 * 1024
	MaxPublishedReceipts = 1000
	MaxReceiptEntries = 1024
)

var ErrReceiptUnavailable = errors.New("sanitized receipt publication unavailable")

// Receipt is an exact allowlist of bounded, journal-derived fields. There
// are no free-form messages, paths, source configs, stdout or credentials.
// The receipt is informational, never deployment evidence or authorization.
type Receipt struct {
	SchemaVersion int `json:"schema_version"`
	OperationID string `json:"operation_id"`
	ProposalID string `json:"proposal_id"`
	RegistrationID string `json:"registration_id"`
	Service string `json:"service"`
	Phase string `json:"phase"`
	Outcome string `json:"outcome"`
	CandidateDigest string `json:"candidate_digest"`
	UpdatedAt string `json:"updated_at"`
	MessageCode string `json:"message_code"`
}

type ReceiptPublication struct {
	ContentSHA256 string
	// FilePublished never implies actual operation success. It only
	// establishes that the exact sanitized receipt is durably on disk.
	FilePublished bool
	AlreadyPublished bool
	Outcome string
}

func receiptFromSnapshot(snap Snapshot) (Receipt,error) {
	var zero Receipt
	op:=snap.Operation
	if !executorregistration.ValidID(op.ID) ||
		!executorregistration.ValidID(op.ProposalID) ||
		!executorregistration.ValidID(op.RegistrationID) ||
		!executorregistration.ValidID(op.ServiceID) ||
		!validHash(op.CandidateDigest) || !validAt(op.UpdatedAt) {
		return zero,ErrReceiptUnavailable
	}
	if op.CurrentPhase!="NONE" && phaseIndex(op.CurrentPhase)<0 {
		return zero,ErrReceiptUnavailable
	}
	outcome:="pending"
	code:="operation_pending"
	if snap.EffectiveOutcome==OutcomeNeedsReconciliation ||
		op.Outcome==OutcomeNeedsReconciliation {
		outcome=OutcomeNeedsReconciliation
		code="verification_incomplete"
		if len(snap.Events)>0 && snap.Events[len(snap.Events)-1].Kind==EventIntent {
			code="interrupted"
		}
	} else if op.Outcome!=OutcomePending {
		// No Phase 3 operation-success verification gate has been accepted.
		// An external SQL "succeeded" claim must NOT reach the public GUI.
		return zero,ErrReceiptUnavailable
	}
	r:=Receipt{
		SchemaVersion:ReceiptSchemaVersion, OperationID:op.ID,
		ProposalID:op.ProposalID,RegistrationID:op.RegistrationID,
		Service:op.ServiceID,Phase:op.CurrentPhase,Outcome:outcome,
		CandidateDigest:op.CandidateDigest,
		UpdatedAt:op.UpdatedAt.UTC().Format(time.RFC3339Nano),
		MessageCode:code,
	}
	if _,err:=r.canonical();err!=nil {return zero,ErrReceiptUnavailable}
	return r,nil
}

func (r Receipt) canonical() ([]byte,error) {
	if r.SchemaVersion!=ReceiptSchemaVersion ||
		!executorregistration.ValidID(r.OperationID) ||
		!executorregistration.ValidID(r.ProposalID) ||
		!executorregistration.ValidID(r.RegistrationID) ||
		!executorregistration.ValidID(r.Service) ||
		(r.Phase!="NONE" && phaseIndex(r.Phase)<0) ||
		(r.Outcome!="pending" && r.Outcome!=OutcomeNeedsReconciliation && r.Outcome!="succeeded") ||
		!validHash(r.CandidateDigest) ||
		(r.MessageCode!="operation_pending" && r.MessageCode!="interrupted" &&
			r.MessageCode!="verification_incomplete" && r.MessageCode!="verified_update") {
		return nil,ErrReceiptUnavailable
	}
	if r.Outcome=="pending" && r.MessageCode!="operation_pending" ||
		r.Outcome==OutcomeNeedsReconciliation && r.MessageCode=="operation_pending" ||\n  r.Outcome=="succeeded" && r.MessageCode!="verified_update" ||\n  r.Outcome!="succeeded" && r.MessageCode=="verified_update" {
		return nil,ErrReceiptUnavailable
	}
	ts,err:=time.Parse(time.RFC3339Nano,r.UpdatedAt)
	if err!=nil || !validAt(ts) ||
		ts.UTC().Format(time.RFC3339Nano)!=r.UpdatedAt {return nil,ErrReceiptUnavailable}
	data,err:=json.Marshal(r)
	if err!=nil || len(data)==0 || len(data)>MaxReceiptBytes {return nil,ErrReceiptUnavailable}
	return data,nil
}

// ParseReceipt is a strict, read-only future-monitor parser. It accepts
// exactly canonical v1 JSON, preventing duplicate keys, extra members,
// trailing tokens and alternative status spellings from becoming GUI data.
func ParseReceipt(data []byte) (Receipt,error) {
	var r Receipt
	if len(data)==0 || len(data)>MaxReceiptBytes {return Receipt{},ErrReceiptUnavailable}
	if err:=json.Unmarshal(data,&r);err!=nil{return Receipt{},ErrReceiptUnavailable}
	canonical,err:=r.canonical()
	if err!=nil || !bytes.Equal(data,canonical){return Receipt{},ErrReceiptUnavailable}
	return r,nil
}

func receiptHash(data []byte) string {
	sum:=sha256.Sum256(data)
	return "sha256:"+hex.EncodeToString(sum[:])
}

// journalReceiptSnapshot reads the operation and events in one immutable
// SQLite snapshot; it does not require or assume a Docker-backed reader.
// The caller holds Store.mu.
func (s *Store) journalReceiptSnapshot(ctx context.Context, id string) (Receipt,error) {
	if s==nil || s.db==nil || ctx==nil || ctx.Err()!=nil ||
		!executorregistration.ValidID(id) {return Receipt{},ErrReceiptUnavailable}
	tx,err:=s.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true})
	if err!=nil{return Receipt{},ErrReceiptUnavailable}
	defer tx.Rollback()
	snap,err:=inspectJournalTx(ctx,tx,id)
	if err!=nil{return Receipt{},ErrReceiptUnavailable}
	return receiptFromSnapshot(snap)
}

// reserveReceipt records an immutable pending publication BEFORE any file
// mutation. An existing same-version hash is an idempotent retry; a
// mismatched hash, version, or uncertain row blocks the operation.
func (s *Store) reserveReceipt(ctx context.Context,id,hash string,at time.Time) (bool,error) {
	if s==nil || ctx==nil || ctx.Err()!=nil || !validHash(hash) || !validAt(at) {
		return false,ErrReceiptUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db==nil{return false,ErrReceiptUnavailable}
	receipt,err:=s.journalReceiptSnapshot(ctx,id)
	if err!=nil{return false,ErrReceiptUnavailable}
	data,err:=receipt.canonical()
	if err!=nil || receiptHash(data)!=hash {return false,ErrReceiptUnavailable}
	tx,err:=s.db.BeginTx(ctx,nil)
	if err!=nil{return false,ErrReceiptUnavailable}
	defer tx.Rollback()
	rows,err:=tx.QueryContext(ctx,`SELECT content_sha256,publication_state
		FROM receipt_publications WHERE operation_id=? AND receipt_version=1`,id)
	if err!=nil{return false,ErrReceiptUnavailable}
	pending,published:=false,false
	for rows.Next() {
		var existing,state string
		if rows.Scan(&existing,&state)!=nil || existing!=hash {rows.Close();return false,ErrReceiptUnavailable}
		switch state {
		case "pending":pending=true
		case "published":published=true
		default:rows.Close();return false,ErrReceiptUnavailable
		}
	}
	rowErr:=rows.Err()
	rows.Close()
	if rowErr!=nil || published && !pending{return false,ErrReceiptUnavailable}
	if !pending {
		_,err=tx.ExecContext(ctx,`INSERT INTO receipt_publications
			(operation_id,receipt_version,content_sha256,publication_state,recorded_ns)
			VALUES (?,1,?,'pending',?)`,id,hash,at.UnixNano())
		if err!=nil{return false,ErrReceiptUnavailable}
	}
	if tx.Commit()!=nil{return false,ErrReceiptUnavailable}
	return published,nil
}

// markReceiptPublished confirms the same still-current snapshot and pending
// hash before recording a published event. It never modifies operations,
// closes a pending intent or declares a Docker/recovery success.
func (s *Store) markReceiptPublished(ctx context.Context,id,hash string,at time.Time) error {
	if s==nil || ctx==nil || ctx.Err()!=nil || !validHash(hash) || !validAt(at) {
		return ErrReceiptUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db==nil{return ErrReceiptUnavailable}
	receipt,err:=s.journalReceiptSnapshot(ctx,id)
	if err!=nil{return ErrReceiptUnavailable}
	data,err:=receipt.canonical()
	if err!=nil || receiptHash(data)!=hash{return ErrReceiptUnavailable}
	tx,err:=s.db.BeginTx(ctx,nil)
	if err!=nil{return ErrReceiptUnavailable}
	defer tx.Rollback()
	var existing string
	err=tx.QueryRowContext(ctx,`SELECT content_sha256 FROM receipt_publications
		WHERE operation_id=? AND receipt_version=1 AND publication_state='pending'`,id).
		Scan(&existing)
	if err!=nil || existing!=hash{return ErrReceiptUnavailable}
	var published int
	err=tx.QueryRowContext(ctx,`SELECT count(*) FROM receipt_publications
		WHERE operation_id=? AND receipt_version=1 AND publication_state='published'
		AND content_sha256=?`,id,hash).Scan(&published)
	if err!=nil || published>1{return ErrReceiptUnavailable}
	if published==0 {
		_,err=tx.ExecContext(ctx,`INSERT INTO receipt_publications
			(operation_id,receipt_version,content_sha256,publication_state,recorded_ns)
			VALUES (?,1,?,'published',?)`,id,hash,at.UnixNano())
		if err!=nil{return ErrReceiptUnavailable}
	}
	if tx.Commit()!=nil{return ErrReceiptUnavailable}
	return nil
}

// ReceiptState is an inspection-only journal view: absence or inconsistency
// must be shown as unknown/pending, never an inferred success.
func (s *Store) ReceiptState(ctx context.Context,id string) (string,error) {
	if s==nil || ctx==nil || ctx.Err()!=nil || !executorregistration.ValidID(id) {
		return "",ErrReceiptUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db==nil{return "",ErrReceiptUnavailable}
	rows,err:=s.db.QueryContext(ctx,`SELECT publication_state,content_sha256
		FROM receipt_publications WHERE operation_id=? AND receipt_version=1`,id)
	if err!=nil{return "",ErrReceiptUnavailable}
	defer rows.Close()
	pending,published:=false,false
	var hash string
	for rows.Next() {
		var state,h string
		if rows.Scan(&state,&h)!=nil || !validHash(h) ||
			(hash!="" && hash!=h) {return "",ErrReceiptUnavailable}
		hash=h
		switch state {
		case "pending":pending=true
		case "published":published=true
		default:return "",ErrReceiptUnavailable
		}
	}
	if rows.Err()!=nil || published&&!pending{return "",ErrReceiptUnavailable}
	if published{return "published",nil}
	if pending{return "pending",nil}
	return "unknown",nil
}
