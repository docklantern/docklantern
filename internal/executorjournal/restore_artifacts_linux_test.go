package executorjournal

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "strings"
 "syscall"
 "testing"
 "time"
)

func artifactAuditFixture(t *testing.T)(*Store,string,string,string,string,string){
 t.Helper()
 backupRootEnabled(t)
 root,e:=os.MkdirTemp("/root","docklantern-artifact-audit-")
 if e!=nil{t.Fatal(e)}
 t.Cleanup(func(){_ = os.RemoveAll(root)})
 if e=os.Chmod(root,0o700);e!=nil{t.Fatal(e)}
 for _,spec:=range []struct{name string;mode os.FileMode}{{"overrides",0o700},{"receipts",0o755},{"journal",0o700}} {
  p:=filepath.Join(root,spec.name)
  if e=os.Mkdir(p,spec.mode);e!=nil{t.Fatal(e)}
  if e=os.Chmod(p,spec.mode);e!=nil{t.Fatal(e)}
 }
 overrides:=filepath.Join(root,"overrides")
 receipts:=filepath.Join(root,"receipts")
 projectDir:=filepath.Join(overrides,"audit-project")
 if e=os.Mkdir(projectDir,0o700);e!=nil{t.Fatal(e)}
 s,e:=openSQLite(filepath.Join(root,"journal","operations.sqlite"))
 if e!=nil{t.Fatal(e)}
 t.Cleanup(func(){_ = s.Close()})
 ctx:=context.Background()
 op:=operationFixture("audit-op","audit-project")
 if e=s.Begin(ctx,op);e!=nil{t.Fatal(e)}
 if e=s.Intent(ctx,op.ID,"PREPARE",fixedTime.Add(time.Second));e!=nil{t.Fatal(e)}
 if e=s.MarkUncertain(ctx,op.ID,"PREPARE","interrupted",fixedTime.Add(2*time.Second));e!=nil{t.Fatal(e)}
 m:=OverrideManifest{SchemaVersion:1,ProjectID:"audit-project",OperationID:"audit-op",
  Generation:1,PreviousGeneration:0,PreviousCommittedSHA256:"sha256:"+strings.Repeat("a",64),
  CandidateImageDigest:"sha256:"+strings.Repeat("b",64)}
 bytes,e:=m.bytes()
 if e!=nil{t.Fatal(e)}
 digest:=sha256.Sum256(bytes)
 hash:="sha256:"+hex.EncodeToString(digest[:])
 if _,e=s.db.ExecContext(ctx,`INSERT INTO override_generation_events
 (project_id,generation,operation_id,event_sequence,event_kind,manifest_sha256,
 candidate_digest,previous_generation,previous_committed_sha256,recorded_ns)
 VALUES (?,?,?,?,?,?,?,?,?,?)`,
 "audit-project",1,"audit-op",1,"stage_intent",hash,m.CandidateImageDigest,
 0,m.PreviousCommittedSHA256,fixedTime.Add(3*time.Second).UnixNano());e!=nil{t.Fatal(e)}
 overrideFD,e:=openOverrideProjectDir(overrides,"audit-project")
 if e!=nil{t.Fatal(e)}
 if e=publishImmutableManifest(overrideFD,1,bytes);e!=nil{t.Fatal(e)}
 syscall.Close(overrideFD)
 receipt,e:=s.receiptForOperation(ctx,"audit-op")
 if e!=nil{t.Fatal(e)}
 body,e:=receipt.canonical()
 if e!=nil{t.Fatal(e)}
 receiptHash:=receiptHash(body)
 receiptFD,e:=openReceiptOutbox(receipts)
 if e!=nil{t.Fatal(e)}
 if e=publishOrVerifyReceiptFile(receiptFD,"audit-op-v1.json",body,receiptHash);e!=nil{t.Fatal(e)}
 syscall.Close(receiptFD)
 for _,state:=range []string{"pending","published"}{
  if _,e=s.db.ExecContext(ctx,`INSERT INTO receipt_publications
  (operation_id,receipt_version,content_sha256,publication_state,recorded_ns)
  VALUES (?,1,?,?,?)`,"audit-op",receiptHash,state,fixedTime.Add(4*time.Second).UnixNano());e!=nil{t.Fatal(e)}
 }
 candidate:=filepath.Join(root,"snapshot.sqlite")
 if _,e=s.db.ExecContext(ctx,"VACUUM INTO '"+candidate+"'");e!=nil{t.Fatal(e)}
 if e=verifyBackupDB(ctx,candidate);e!=nil{t.Fatal(e)}
 return s,candidate,overrides,receipts,filepath.Join(projectDir,overrideFilename(1)),filepath.Join(receipts,"audit-op-v1.json")
}

func TestArtifactAuditMatchesNeverAuthorizesRootFixture(t *testing.T){
 _,candidate,overrideRoot,receiptDir,_,_:=artifactAuditFixture(t)
 result,e:=auditRestoredArtifactReferencesAt(context.Background(),candidate,overrideRoot,receiptDir)
 if e!=nil||result.Status!=RestoreRowsMatchUnqualified||
  result.ReasonCode!="referenced_artifacts_verified_only"||!result.BlocksNewWork{
  t.Fatalf("matching external evidence authorized work: %#v %v",result,e)
 }
}

func TestArtifactAuditDetectsMissingAndTamperedEvidenceRootFixture(t *testing.T){
 cases:=[]struct{name string;attack func(*testing.T,string,string)}{
  {"missing_override",func(t *testing.T,override,receipt string){if e:=os.Remove(override);e!=nil{t.Fatal(e)}}},
  {"tampered_override",func(t *testing.T,override,receipt string){if e:=os.WriteFile(override,[]byte("corrupt"),0o600);e!=nil{t.Fatal(e)}}},
  {"symlink_override",func(t *testing.T,override,receipt string){
   if e:=os.Remove(override);e!=nil{t.Fatal(e)}
   if e:=os.Symlink("/etc/passwd",override);e!=nil{t.Fatal(e)}
  }},
  {"missing_receipt",func(t *testing.T,override,receipt string){if e:=os.Remove(receipt);e!=nil{t.Fatal(e)}}},
  {"tampered_receipt",func(t *testing.T,override,receipt string){if e:=os.WriteFile(receipt,[]byte("corrupt"),0o644);e!=nil{t.Fatal(e)}}},
  {"symlink_receipt",func(t *testing.T,override,receipt string){
   if e:=os.Remove(receipt);e!=nil{t.Fatal(e)}
   if e:=os.Symlink("/etc/passwd",receipt);e!=nil{t.Fatal(e)}
  }},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  _,candidate,overrides,receipts,overrideFile,receiptFile:=artifactAuditFixture(t)
  tc.attack(t,overrideFile,receiptFile)
  result,e:=auditRestoredArtifactReferencesAt(context.Background(),candidate,overrides,receipts)
  if e!=nil||result.Status!=RestoreNeedsReconciliation||!result.BlocksNewWork{
   t.Fatalf("unsafe external state accepted: %#v %v",result,e)
  }
 })}
}

func TestArtifactAuditPendingReceiptRemainsUnresolvedRootFixture(t *testing.T){
 s,_,overrides,receipts,_,_:=artifactAuditFixture(t)
 if _,e:=s.db.ExecContext(context.Background(),`DELETE FROM receipt_publications WHERE publication_state='published'`);e==nil {
  t.Fatal("append-only journal allowed deletion")
 }
 // Construct an independent older snapshot after the pending row, without
 // changing the immutable authoritative fixture or its publish history.
 pending:=filepath.Join(filepath.Dir(receipts),"pending.sqlite")
 if _,e:=s.db.ExecContext(context.Background(),"VACUUM INTO '"+pending+"'");e!=nil{t.Fatal(e)}
 // Database triggers intentionally forbid deletion. Create a new disposable
 // SQLite candidate by adding a pending row to a distinct operation instead.
 // A pending only fixture already has a final file after interrupted
 // publication; it must not be interpreted as a committed publication.
 op:=operationFixture("pending-only","other-audit-project")
 if e:=s.Begin(context.Background(),op);e!=nil{t.Fatal(e)}
 hash:="sha256:"+strings.Repeat("c",64)
 if _,e:=s.db.Exec(`INSERT INTO receipt_publications
  (operation_id,receipt_version,content_sha256,publication_state,recorded_ns)
  VALUES (?,1,?,'pending',?)`,"pending-only",hash,fixedTime.Add(time.Minute).UnixNano());e!=nil{t.Fatal(e)}
 later:=filepath.Join(filepath.Dir(receipts),"pending-later.sqlite")
 if _,e:=s.db.ExecContext(context.Background(),"VACUUM INTO '"+later+"'");e!=nil{t.Fatal(e)}
 result,e:=auditRestoredArtifactReferencesAt(context.Background(),later,overrides,receipts)
 if e!=nil||result.Status!=RestoreNeedsReconciliation||!result.BlocksNewWork{
  t.Fatalf("pending-only outbox journal accepted: %#v %v",result,e)
 }
}
