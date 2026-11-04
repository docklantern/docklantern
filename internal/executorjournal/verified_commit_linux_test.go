package executorjournal

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestVerifiedCommitRootFixture(t *testing.T){
 if os.Geteuid()!=0||os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1"{
  t.Skip("isolated protected journal fixture only")
 }
 ctx:=context.Background()
 root,err:=os.MkdirTemp("/root","docklantern-finalize-")
 if err!=nil{t.Fatal(err)}
 defer os.RemoveAll(root)
 if err=os.Chmod(root,0o700);err!=nil{t.Fatal(err)}
 locks:=filepath.Join(root,"locks")
 if err=os.Mkdir(locks,0o700);err!=nil{t.Fatal(err)}
 op:=operationFixture("verify-operation","verify-project")
 lock,err:=acquireProjectLockAt(locks,op.ProjectID)
 if err!=nil{t.Fatal(err)}
 defer lock.Release()
 store,_:=journalFixture(t)
 if err=store.Begin(ctx,op);err!=nil{t.Fatal(err)}
 for i,phase:=range []string{"PREPARE","AUTHORIZE","PREFLIGHT","PULL","STAGE","APPLY","VERIFY"}{
  at:=op.CreatedAt.Add(time.Duration(2*i+1)*time.Second)
  if err=store.Intent(ctx,op.ID,phase,at);err!=nil{t.Fatalf("intent %s: %v",phase,err)}
  if err=store.Complete(ctx,op.ID,phase,at.Add(time.Second));err!=nil{t.Fatalf("complete %s: %v",phase,err)}
 }
 intentAt:=op.CreatedAt.Add(15*time.Second)
 if err=store.Intent(ctx,op.ID,"COMMIT",intentAt);err!=nil{t.Fatal(err)}
 hash:=func(ch string)string{return "sha256:"+strings.Repeat(ch,64)}
 proof:=VerifiedEvidence{
  ProjectID:op.ProjectID,ServiceID:op.ServiceID,
  CandidateDigest:op.CandidateDigest,SourceSHA256:hash("c"),
  BaselineContainerID:strings.Repeat("a",64),UpdatedContainerID:strings.Repeat("b",64),
  BaselineImageID:hash("d"),UpdatedImageID:hash("e"),
  HealthGrade:"healthy",SiblingsUnchanged:true,
 }
 if err=store.CommitVerified(ctx,lock,op.ID,proof,intentAt.Add(time.Second));err!=nil{
  t.Fatalf("verified journal commit denied: %v",err)
 }
 snap,err:=store.Inspect(ctx,op.ID)
 if err!=nil||snap.Operation.Outcome!="succeeded"||snap.BlocksNewWork||
  len(snap.Events)!=16||snap.Events[15].Kind!=EventCompleted{
  t.Fatalf("committed outcome missing: %#v %v",snap,err)
 }
 var n int
 if err=store.db.QueryRowContext(ctx,`SELECT count(*) FROM override_records
  WHERE project_id=?`,op.ProjectID).Scan(&n);err!=nil||n!=2{
  t.Fatalf("desired generation not durably recorded: %d %v",n,err)
 }
 if _,err=store.receiptForOperation(ctx,op.ID);err!=nil{
  t.Fatalf("verified success receipt withheld: %v",err)
 }
 newer:=operationFixture("verify-next-operation",op.ProjectID)
 if err=store.Begin(ctx,newer);err!=nil{
  t.Fatalf("verified commit did not unblock subsequent approved operations: %v",err)
 }
}

func TestVerifiedCommitRejectsAbsentWitnessRootFixture(t *testing.T){
 if os.Geteuid()!=0||os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1"{
  t.Skip("isolated protected journal fixture only")
 }
 ctx:=context.Background()
 root,err:=os.MkdirTemp("/root","docklantern-denial-")
 if err!=nil{t.Fatal(err)}
 defer os.RemoveAll(root)
 os.Chmod(root,0o700)
 locks:=filepath.Join(root,"locks")
 os.Mkdir(locks,0o700)
 op:=operationFixture("verify-denied","verify-project")
 lock,err:=acquireProjectLockAt(locks,op.ProjectID)
 if err!=nil{t.Fatal(err)}
 defer lock.Release()
 store,_:=journalFixture(t)
 if err=store.Begin(ctx,op);err!=nil{t.Fatal(err)}
 proof:=VerifiedEvidence{ProjectID:op.ProjectID,ServiceID:op.ServiceID,
  CandidateDigest:op.CandidateDigest,SourceSHA256:"sha256:"+strings.Repeat("c",64),
  BaselineContainerID:strings.Repeat("a",64),UpdatedContainerID:strings.Repeat("b",64),
  BaselineImageID:"sha256:"+strings.Repeat("d",64),
  UpdatedImageID:"sha256:"+strings.Repeat("e",64),HealthGrade:"healthy",
  SiblingsUnchanged:false,
 }
 if store.CommitVerified(ctx,lock,op.ID,proof,op.CreatedAt.Add(time.Second))==nil{
  t.Fatal("no sibling witness authorized commit")
 }
 snap,err:=store.Inspect(ctx,op.ID)
 if err!=nil||snap.Operation.Outcome!="pending"||!snap.BlocksNewWork{
  t.Fatal("failed witness incorrectly finalized operation")
 }
}
