package executorjournal

import (
 "context"
 "database/sql"
 "os"
 "syscall"
)

// auditRestoredArtifactReferencesAt verifies filesystem evidence that a
// candidate journal *claims* was staged or published. It never authorizes a
// restore: matching referenced files cannot rule out unrecorded newer files,
// later real Docker/Compose changes, or independent writers.
//
// Path-taking helper is private to disposable root fixtures. No production
// restore path, monitor, web or CLI calls it.
func auditRestoredArtifactReferencesAt(ctx context.Context, candidatePath, overrideRoot, receiptDir string) (RestoreComparison,error) {
 blocked:=RestoreComparison{Status:RestoreNeedsReconciliation,ReasonCode:"external_evidence_unavailable",BlocksNewWork:true}
 if ctx==nil||ctx.Err()!=nil||os.Geteuid()!=0||candidatePath=="" {
  return blocked,ErrUnavailable
 }
 if err:=verifyBackupDB(ctx,candidatePath);err!=nil{return blocked,ErrUnavailable}
 db,err:=sql.Open("sqlite","file:"+candidatePath+"?mode=ro&immutable=1")
 if err!=nil{return blocked,ErrUnavailable}
 defer db.Close()
 db.SetMaxOpenConns(1)
 tx,err:=db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true})
 if err!=nil{return blocked,ErrUnavailable}
 defer tx.Rollback()

 // Stage intents can exist without a durable file after interruption.
 // Even when a file exists, such events are never committed desired state.
 rows,err:=tx.QueryContext(ctx, `SELECT project_id,generation,manifest_sha256
  FROM override_generation_events ORDER BY project_id,generation,event_sequence`)
 if err!=nil{return blocked,ErrUnavailable}
 count:=0
 for rows.Next(){
  count++
  if count>maxRestoreComparisonRows{rows.Close();return blocked,ErrUnavailable}
  var project,hash string
  var generation int64
  if rows.Scan(&project,&generation,&hash)!=nil || !validHash(hash){
   rows.Close();return blocked,ErrUnavailable
  }
  fd,e:=openOverrideProjectDir(overrideRoot,project)
  if e!=nil {rows.Close();blocked.ReasonCode="override_artifact_missing_or_untrusted";return blocked,nil}
  e=verifyManifest(fd,generation,hash)
  syscall.Close(fd)
  if e!=nil {
   rows.Close()
   blocked.ReasonCode="override_artifact_missing_or_untrusted"
   return blocked,nil
  }
 }
 err=rows.Err()
 rows.Close()
 if err!=nil{return blocked,ErrUnavailable}

 // Pending publications are deliberately unresolved; never infer that a
 // pending outbox write actually reached the filesystem.
 receipts,err:=tx.QueryContext(ctx,`SELECT operation_id,content_sha256,publication_state
  FROM receipt_publications ORDER BY operation_id,receipt_version,publication_state`)
 if err!=nil{return blocked,ErrUnavailable}
 var dirfd int = -1
 defer func(){if dirfd>=0{syscall.Close(dirfd)}}()
 count=0
 states:=make(map[string]uint8)
 for receipts.Next(){
  count++
  if count>maxRestoreComparisonRows{receipts.Close();return blocked,ErrUnavailable}
  var operationID,hash,state string
  if receipts.Scan(&operationID,&hash,&state)!=nil||!validHash(hash) {
   receipts.Close();return blocked,ErrUnavailable
  }
  if state!="pending"&&state!="published" {
   receipts.Close();blocked.ReasonCode="receipt_state_unresolved";return blocked,nil
  }
  if dirfd<0 {
   dirfd,err=openReceiptOutbox(receiptDir)
   if err!=nil {
    receipts.Close()
    blocked.ReasonCode="receipt_artifact_missing_or_untrusted"
    return blocked,nil
   }
  }
  e:=verifyReceiptFile(dirfd,operationID+"-v1.json",hash)
  if e!=nil {
   receipts.Close()
   blocked.ReasonCode="receipt_artifact_missing_or_untrusted"
   return blocked,nil
  }
  key:=operationID+"|"+hash
  if state=="pending"{states[key]|=1}else{states[key]|=2}
 }
 err=receipts.Err()
 receipts.Close()
 if err!=nil{return blocked,ErrUnavailable}
 for _,bits:=range states {
  if bits!=3 {blocked.ReasonCode="receipt_state_unresolved";return blocked,nil}
 }
 if ctx.Err()!=nil{return blocked,ErrUnavailable}
 return RestoreComparison{Status:RestoreRowsMatchUnqualified,
  ReasonCode:"referenced_artifacts_verified_only",BlocksNewWork:true},nil
}

