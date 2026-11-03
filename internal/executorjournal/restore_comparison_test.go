package executorjournal

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func restoreComparisonFixture(t *testing.T) (*Store,string) {
 t.Helper()
 s,_:=journalFixture(t)
 ctx:=context.Background()
 op:=operationFixture("compare-op","compare-project")
 if err:=s.Begin(ctx,op);err!=nil{t.Fatal(err)}
 if err:=s.Intent(ctx,op.ID,"PREPARE",fixedTime.Add(time.Second));err!=nil{t.Fatal(err)}
 backup:=filepath.Join(t.TempDir(),"before.sqlite")
 // Only a disposable CI DB. Production backup uses the separately
 // root-protected filesystem and verified manifest path.
 if _,err:=s.db.ExecContext(ctx,"VACUUM INTO '"+backup+"'");err!=nil{t.Fatal(err)}
 if err:=verifyBackupDB(ctx,backup);err!=nil{t.Fatal(err)}
 return s,backup
}

func TestRestoreComparisonExactDatabaseMatchNeverAuthorizes(t *testing.T){
 s,path:=restoreComparisonFixture(t)
 report,err:=s.compareRestoreJournal(context.Background(),path)
 if err!=nil||report.Status!=RestoreRowsMatchUnqualified||
  report.ReasonCode!="database_rows_match_only"||!report.BlocksNewWork{
  t.Fatalf("matching snapshot granted authority: %#v %v",report,err)
 }
}

func TestRestoreComparisonDetectsNewerEvidence(t *testing.T){
 cases:=[]struct{name string;change func(*testing.T,*Store)}{
  {"phase_event",func(t *testing.T,s *Store){
   if e:=s.MarkUncertain(context.Background(),"compare-op","PREPARE","interrupted",fixedTime.Add(2*time.Second));e!=nil{t.Fatal(e)}
  }},
  {"committed_override",func(t *testing.T,s *Store){
   _,e:=s.db.Exec("INSERT INTO override_records(project_id,generation,committed_sha256) VALUES(?,?,?)",
    "compare-project",1,"sha256:"+strings.Repeat("a",64))
   if e!=nil{t.Fatal(e)}
  }},
  {"generation_intent",func(t *testing.T,s *Store){
   hash:="sha256:"+strings.Repeat("a",64)
   _,e:=s.db.Exec(`INSERT INTO override_generation_events
     (project_id,generation,operation_id,event_sequence,event_kind,manifest_sha256,
      candidate_digest,previous_generation,previous_committed_sha256,recorded_ns)
      VALUES(?,1,?,1,'stage_intent',?,?,0,?,?)`,
      "compare-project","compare-op",hash,"sha256:"+strings.Repeat("b",64),hash,fixedTime.Add(time.Minute).UnixNano())
   if e!=nil{t.Fatal(e)}
  }},
  {"receipt_publication",func(t *testing.T,s *Store){
   _,e:=s.db.Exec(`INSERT INTO receipt_publications
     (operation_id,receipt_version,content_sha256,publication_state,recorded_ns)
     VALUES(?,1,?,'pending',?)`,
     "compare-op","sha256:"+strings.Repeat("c",64),fixedTime.Add(time.Minute).UnixNano())
   if e!=nil{t.Fatal(e)}
  }},
  {"newer_operation",func(t *testing.T,s *Store){
   op:=operationFixture("newer-op","other-project")
   if e:=s.Begin(context.Background(),op);e!=nil{t.Fatal(e)}
  }},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   s,path:=restoreComparisonFixture(t)
   tc.change(t,s)
   report,e:=s.compareRestoreJournal(context.Background(),path)
   if e!=nil || report.Status!=RestoreNeedsReconciliation||
    report.ReasonCode!="journal_history_diverged" || !report.BlocksNewWork {
    t.Fatalf("older journal accepted: %#v %v",report,e)
   }
  })
 }
}

func TestRestoreComparisonFailsClosedOnInvalidBackup(t *testing.T){
 s,path:=restoreComparisonFixture(t)
 if err:=os.WriteFile(path,[]byte("not sqlite"),0o600);err!=nil{t.Fatal(err)}
 report,err:=s.compareRestoreJournal(context.Background(),path)
 if !errors.Is(err,ErrUnavailable)||!report.BlocksNewWork||
  report.Status!=RestoreNeedsReconciliation{
  t.Fatalf("corrupt snapshot accepted: %#v %v",report,err)
 }
}
