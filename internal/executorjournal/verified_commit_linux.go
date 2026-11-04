package executorjournal

import (
 "context"
 "database/sql"
 "errors"
 "os"
 "time"
)

var ErrVerifiedCommit=errors.New("independently verified commit unavailable")

// VerifiedEvidence is a bounded witness from the trusted host-native
// read-after-write inspector, not from Docker Compose stdout, GUI, proposal,
// registry watch, or monitor. A running-only service must be explicitly
// allowed by the protected registration.
type VerifiedEvidence struct {
 ProjectID string
 ServiceID string
 SourceSHA256 string
 CandidateDigest string
 BaselineContainerID string
 BaselineImageID string
 UpdatedContainerID string
 UpdatedImageID string
 HealthGrade string // healthy or running_only
 SiblingsUnchanged bool
}

// CommitVerified is the sole v1 success writer. The caller MUST hold the
// original project lock through terminal approval, pull, staged Compose up,
// post-update Docker inspection and this transaction. It atomically records
// the verified generation, a COMMIT completion event and operation outcome.
// The journal never interprets an ordinary phase Complete as success.
func (s *Store)CommitVerified(ctx context.Context,lock *ProjectLock,id string,
 proof VerifiedEvidence,at time.Time)error{
 if s==nil||lock==nil||ctx==nil||ctx.Err()!=nil||os.Geteuid()!=0||
  !validAt(at)||!validHash(proof.SourceSHA256)||
  !validHash(proof.CandidateDigest)||!validHash(proof.BaselineImageID)||
  !validHash(proof.UpdatedImageID)||!validFullContainerID(proof.BaselineContainerID)||
  !validFullContainerID(proof.UpdatedContainerID)||
  proof.BaselineContainerID==proof.UpdatedContainerID||
  !proof.SiblingsUnchanged||
  (proof.HealthGrade!="healthy"&&proof.HealthGrade!="running_only"){
  return ErrVerifiedCommit
 }
 lock.mu.Lock()
 defer lock.mu.Unlock()
 if lock.released||lock.fd<0||lock.projectID!=proof.ProjectID||
  !trustedJournalFile(lock.fd){return ErrVerifiedCommit}
 s.mu.Lock()
 defer s.mu.Unlock()
 if s.db==nil||s.inFlight[id]!="COMMIT"{return ErrVerifiedCommit}
 tx,err:=s.db.BeginTx(ctx,nil)
 if err!=nil{return ErrVerifiedCommit}
 defer tx.Rollback()
 var project,service,candidate,state string
 var createdNS,updatedNS int64
 err=tx.QueryRowContext(ctx,`SELECT project_id,service_id,candidate_digest,
  outcome,created_ns,updated_ns FROM operations WHERE operation_id=?`,id).
  Scan(&project,&service,&candidate,&state,&createdNS,&updatedNS)
 if err!=nil||project!=proof.ProjectID||service!=proof.ServiceID||
  candidate!=proof.CandidateDigest||state!=OutcomePending||
  at.UnixNano()<updatedNS||at.UnixNano()<createdNS{
  return ErrVerifiedCommit
 }
 previous,found,err:=last(tx,ctx,id)
 if err!=nil||!found||previous.Seq!=15||
  previous.Kind!=EventIntent||previous.Phase!="COMMIT"||
  previous.AtNS>at.UnixNano(){return ErrVerifiedCommit}
 var staged int
 if tx.QueryRowContext(ctx,`SELECT count(*) FROM override_generation_events
  WHERE project_id=?`,project).Scan(&staged)!=nil||staged!=0{
  return ErrVerifiedCommit
 }
 // The initial generation records the last trusted Compose source identity.
 // Future generations record the independently verified desired digest.
 var prevGen int64
 var prevHash string
 err=tx.QueryRowContext(ctx,`SELECT generation,committed_sha256 FROM
  override_records WHERE project_id=? ORDER BY generation DESC LIMIT 1`,project).
  Scan(&prevGen,&prevHash)
 switch{
 case errors.Is(err,sql.ErrNoRows):
  prevGen=0
  prevHash=proof.SourceSHA256
  if _,err=tx.ExecContext(ctx,`INSERT INTO override_records
   (project_id,generation,committed_sha256)VALUES(?,0,?)`,
   project,prevHash);err!=nil{return ErrVerifiedCommit}
 case err!=nil || !validHash(prevHash):
  return ErrVerifiedCommit
 }
 // Prevent a reconciled newer generation from being accidentally overwritten.
 var blocked int
 if tx.QueryRowContext(ctx,`SELECT count(*) FROM override_records WHERE
  project_id=? AND generation>?`,project,prevGen).Scan(&blocked)!=nil||blocked!=0{
  return ErrVerifiedCommit
 }
 _,err=tx.ExecContext(ctx,`INSERT INTO baselines
  (operation_id,container_id,image_id,platform,source_fingerprint,
   desired_image_fingerprint,health_grade,recorded_ns)
  VALUES(?,?,?,?,?,?,?,?)`,id,proof.BaselineContainerID,proof.BaselineImageID,
  "linux/amd64",proof.SourceSHA256,prevHash,"running_only",at.UnixNano())
 if err!=nil{return ErrVerifiedCommit}
 _,err=tx.ExecContext(ctx,`INSERT INTO override_records
  (project_id,generation,committed_sha256,previous_sha256,
   staged_operation_id,promotion_intent_ns,promotion_complete_ns)
  VALUES(?,?,?,?,?,?,?)`,
  project,prevGen+1,proof.CandidateDigest,prevHash,id,at.UnixNano(),at.UnixNano())
 if err!=nil{return ErrVerifiedCommit}
 _,err=tx.ExecContext(ctx,`INSERT INTO phase_events
  (operation_id,event_sequence,phase,event_kind,event_ns,message_code)
  VALUES (?,16,'COMMIT','completed',?,'none')`,id,at.UnixNano())
 if err!=nil{return ErrVerifiedCommit}
 res,err:=tx.ExecContext(ctx,`UPDATE operations SET current_phase='COMMIT',
  updated_ns=?,outcome='succeeded' WHERE operation_id=? AND outcome='pending'`,
  at.UnixNano(),id)
 if err!=nil{return ErrVerifiedCommit}
 affected,err:=res.RowsAffected()
 if err!=nil||affected!=1{return ErrVerifiedCommit}
 if tx.Commit()!=nil{return ErrVerifiedCommit}
 delete(s.inFlight,id)
 return nil
}
