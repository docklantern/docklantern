package executorjournal

import (
 "context"
 "database/sql"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "syscall"
 "testing"
 "time"
)

func backupFixture(t *testing.T) (*Store,string,string,string) {
 t.Helper()
 base,e:=os.MkdirTemp("/root","docklantern-backup-")
 if e!=nil{t.Fatal(e)}
 t.Cleanup(func(){_ = os.RemoveAll(base)})
 if e=os.Chmod(base,0o700);e!=nil{t.Fatal(e)}
 backup:=filepath.Join(base,"backup")
 quarantine:=filepath.Join(base,"quarantine")
 for _,dir:=range []string{backup,quarantine}{
  if e=os.Mkdir(dir,0o700);e!=nil{t.Fatal(e)}
 }
 source:=filepath.Join(base,"operations.sqlite")
 s,e:=openSQLite(source)
 if e!=nil{t.Fatal(e)}
 t.Cleanup(func(){_ = s.Close()})
 op:=operationFixture("backup-demo","backup-project")
 if e=s.Begin(context.Background(),op);e!=nil{t.Fatal(e)}
 if e=s.Intent(context.Background(),op.ID,"PREPARE",fixedTime.Add(time.Second));e!=nil{t.Fatal(e)}
 return s,source,backup,quarantine
}

func backupRootEnabled(t *testing.T) {
 t.Helper()
 if os.Geteuid()!=0||os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1"{
  t.Skip("requires explicitly enabled disposable root fixture")
 }
}

func TestWALSnapshotAndQuarantinedRestore(t *testing.T) {
 backupRootEnabled(t)
 s,source,backup,quarantine:=backupFixture(t)
 ctx:=context.Background()
 // Never checkpoint the live WAL as a prerequisite for a consistent snapshot.
 if _,e:=os.Stat(source+"-wal");e!=nil{t.Fatalf("expected live WAL: %v",e)}
 id,e:=s.backupJournalAt(ctx,backup,fixedTime.Add(time.Hour),nil)
 if e!=nil{t.Fatalf("consistent WAL backup failed: %v",e)}
 fd,e:=openPrivateBackupDir(backup)
 if e!=nil{t.Fatal(e)}
 if e=verifyBackupBundle(ctx,fd,backup,id);e!=nil{t.Fatal(e)}
 syscall.Close(fd)
 if e=stageRestoreAt(ctx,backup,quarantine,id,nil);e!=nil{t.Fatalf("restore staging failed: %v",e)}
 restored,e:=sqlOpenReadOnlyForTest(filepath.Join(quarantine,"restored-"+id+".sqlite"))
 if e!=nil{t.Fatal(e)}
 defer restored.Close()
 var operation,phase int
 if e=restored.QueryRow("SELECT count(*) FROM operations WHERE operation_id='backup-demo'").Scan(&operation);e!=nil{t.Fatal(e)}
 if e=restored.QueryRow("SELECT count(*) FROM phase_events WHERE operation_id='backup-demo'").Scan(&phase);e!=nil{t.Fatal(e)}
 if operation!=1||phase!=1{t.Fatal("committed WAL history absent from restored quarantine")}
 if _,e=os.Stat(source);e!=nil{t.Fatal("active journal unexpectedly changed",e)}
 if e=stageRestoreAt(ctx,backup,quarantine,id,nil);!errors.Is(e,errBackup){
  t.Fatalf("restore overwrote nonempty quarantine: %v",e)
 }
 // The restored snapshot has no production restore activation or success path.
 snap,e:=s.Inspect(ctx,"backup-demo")
 if e!=nil||snap.EffectiveOutcome!=OutcomeNeedsReconciliation||!snap.BlocksNewWork{
  t.Fatalf("backup cleared an unresolved operation: %#v %v",snap,e)
 }
}

func sqlOpenReadOnlyForTest(path string)(*sql.DB,error){
 return sql.Open("sqlite","file:"+path+"?mode=ro&immutable=1")
}

func TestBackupCorruptionAndUnsafePaths(t *testing.T){
 backupRootEnabled(t)
 s,_,backup,quarantine:=backupFixture(t)
 ctx:=context.Background()
 id,e:=s.backupJournalAt(ctx,backup,fixedTime.Add(time.Hour),nil)
 if e!=nil{t.Fatal(e)}
 name:=filepath.Join(backup,"snapshot-"+id+".sqlite")
 fd,e:=openPrivateBackupDir(backup)
 if e!=nil{t.Fatal(e)}
 if e=verifyBackupBundle(ctx,fd,backup,id);e!=nil{t.Fatal(e)}
 syscall.Close(fd)
 f,e:=os.OpenFile(name,os.O_WRONLY,0)
 if e!=nil{t.Fatal(e)}
 if _,e=f.WriteAt([]byte("corruption"),100);e!=nil{t.Fatal(e)}
 f.Close()
 if e=stageRestoreAt(ctx,backup,quarantine,id,nil);!errors.Is(e,errBackup){
  t.Fatalf("accepted corrupted snapshot: %v",e)
 }
 if e=os.Chmod(backup,0o777);e!=nil{t.Fatal(e)}
 if _,e=s.backupJournalAt(ctx,backup,fixedTime.Add(time.Hour),nil);!errors.Is(e,errBackup){
  t.Fatalf("accepted writable backup directory: %v",e)
 }
}

func TestBackupCrashPointsRemainUntrusted(t *testing.T){
 backupRootEnabled(t)
 for _,point:=range []string{"after_snapshot","after_snapshot_sync","after_backup_rename","after_manifest_sync"}{
  t.Run(point,func(t *testing.T){
   s,_,backup,_:=backupFixture(t)
   _,e:=s.backupJournalAt(context.Background(),backup,fixedTime.Add(time.Hour),func(step string)error{
    if step==point{return errors.New("simulated crash")}
    return nil
   })
   if !errors.Is(e,errBackup){t.Fatalf("crash accepted: %v",e)}
   entries,e:=os.ReadDir(backup)
   if e!=nil{t.Fatal(e)}
   // Only a completely published manifest+database pair can be verified.
   for _,entry:=range entries{
    if strings.HasSuffix(entry.Name(),".json"){
     id:=strings.TrimSuffix(strings.TrimPrefix(entry.Name(),"snapshot-"),".json")
     fd,e:=openPrivateBackupDir(backup)
     if e!=nil{t.Fatal(e)}
     e=verifyBackupBundle(context.Background(),fd,backup,id)
     syscall.Close(fd)
     if e!=nil{t.Fatalf("torn backup presented as valid: %v",e)}
    }
   }
  })
 }
}

func TestRestoreCrashLeavesOnlyQuarantine(t *testing.T){
 backupRootEnabled(t)
 for _,point:=range []string{"after_restore_sync","after_restore_rename"}{
  t.Run(point,func(t *testing.T){
   s,source,backup,quarantine:=backupFixture(t)
   id,e:=s.backupJournalAt(context.Background(),backup,fixedTime.Add(time.Hour),nil)
   if e!=nil{t.Fatal(e)}
   e=stageRestoreAt(context.Background(),backup,quarantine,id,func(step string)error{
    if step==point{return errors.New("simulated restore crash")}
    return nil
   })
   if !errors.Is(e,errBackup){t.Fatalf("restore interruption accepted: %v",e)}
   if _,e=os.Stat(source);e!=nil{t.Fatal("active journal disturbed",e)}
   snapshot,e:=s.Inspect(context.Background(),"backup-demo")
   if e!=nil||!snapshot.BlocksNewWork{t.Fatalf("crash permitted work: %v",e)}
  })
 }
}
