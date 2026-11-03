package executorjournal

import (
 "context"
 "crypto/sha256"
 "database/sql"
 "encoding/hex"
 "encoding/json"
 "reflect"
)

// RestoreComparison is read-only evidence, NEVER an authorization to replace
// the live journal. All outcomes block further work pending independent
// offline, filesystem-generation and actual-state qualification.
type RestoreComparison struct {
 Status string
 ReasonCode string
 BlocksNewWork bool
}

const (
 RestoreNeedsReconciliation = "needs_reconciliation"
 RestoreRowsMatchUnqualified = "rows_match_unqualified"
 maxRestoreComparisonRows = 10000
 maxRestoreRowBytes = 64 << 10
)

// compareRestoreJournal is internal, informational and never callable from
// the monitor or executor CLI. It compares every existing authority-bearing
// SQLite row against a validated, independently snapshotted backup.
//
// Exact equality is intentionally conservative. A candidate that lacks even
// one newer journal event, override or receipt row MUST NOT be interpreted as
// current. Matching database rows still say NOTHING about independent
// filesystem generations, published receipts or actual Docker state.
//
// This holds only the current Store-local mutex, not a cross-process executor
// maintenance lock. It must not be used as an activation or preflight gate.
func (s *Store) compareRestoreJournal(ctx context.Context, candidatePath string) (RestoreComparison,error) {
 blocked:=RestoreComparison{Status:RestoreNeedsReconciliation,ReasonCode:"comparison_unavailable",BlocksNewWork:true}
 if s==nil || ctx==nil || ctx.Err()!=nil || candidatePath=="" {
  return blocked,ErrUnavailable
 }
 if err:=verifyBackupDB(ctx,candidatePath);err!=nil {
  return blocked,ErrUnavailable
 }
 candidate,err:=sql.Open("sqlite","file:"+candidatePath+"?mode=ro&immutable=1")
 if err!=nil{return blocked,ErrUnavailable}
 defer candidate.Close()
 candidate.SetMaxOpenConns(1)
 s.mu.Lock()
 defer s.mu.Unlock()
 if s.db==nil{return blocked,ErrUnavailable}
 // A consistent read transaction prevents multiple SELECTs from seeing
 // different commits on this connection.
 originalTx,err:=s.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true})
 if err!=nil{return blocked,ErrUnavailable}
 defer originalTx.Rollback()
 candidateTx,err:=candidate.BeginTx(ctx,&sql.TxOptions{ReadOnly:true})
 if err!=nil{return blocked,ErrUnavailable}
 defer candidateTx.Rollback()

 for _,table:=range []string{
  "operations","phase_events","baselines","override_records",
  "override_generation_events","receipt_publications",
 } {
  currentRows,err:=journalRowDigests(ctx,originalTx,table)
  if err!=nil{return blocked,ErrUnavailable}
  backupRows,err:=journalRowDigests(ctx,candidateTx,table)
  if err!=nil{return blocked,ErrUnavailable}
  if !reflect.DeepEqual(currentRows,backupRows) {
   blocked.ReasonCode="journal_history_diverged"
   return blocked,nil
  }
 }
 if ctx.Err()!=nil{return blocked,ErrUnavailable}
 return RestoreComparison{
  Status:RestoreRowsMatchUnqualified,
  ReasonCode:"database_rows_match_only",
  BlocksNewWork:true,
 },nil
}

// The table names are a hardcoded allowlist, not caller-provided SQL.
func journalRowDigests(ctx context.Context,tx *sql.Tx,table string) (map[string]int,error) {
 rows,err:=tx.QueryContext(ctx,"SELECT * FROM "+table)
 if err!=nil{return nil,ErrUnavailable}
 defer rows.Close()
 cols,err:=rows.Columns()
 if err!=nil || len(cols)==0 || len(cols)>32{return nil,ErrUnavailable}
 result:=make(map[string]int)
 count:=0
 for rows.Next(){
  count++
  if count>maxRestoreComparisonRows{return nil,ErrUnavailable}
  values:=make([]any,len(cols))
  pointers:=make([]any,len(cols))
  for i:=range values{pointers[i]=&values[i]}
  if err=rows.Scan(pointers...);err!=nil{return nil,ErrUnavailable}
  raw,err:=json.Marshal(values)
  if err!=nil||len(raw)>maxRestoreRowBytes{return nil,ErrUnavailable}
  digest:=sha256.Sum256(raw)
  result[hex.EncodeToString(digest[:])]++
 }
 if rows.Err()!=nil{return nil,ErrUnavailable}
 return result,nil
}
