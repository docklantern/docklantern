package executorjournal

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func TestHostStoreHoldsMaintenanceLeaseRootFixture(t *testing.T) {
 backupRootEnabled(t)
 root,e:=os.MkdirTemp("/root","docklantern-hostlease-")
 if e!=nil{t.Fatal(e)}
 t.Cleanup(func(){_ = os.RemoveAll(root)})
 if e=os.Chmod(root,0o700);e!=nil{t.Fatal(e)}
 journalDir:=filepath.Join(root,"journal")
 lockDir:=filepath.Join(root,"locks")
 for _,dir:=range []string{journalDir,lockDir}{
  if e=os.Mkdir(dir,0o700);e!=nil{t.Fatal(e)}
 }
 // Missing/untrusted lock storage must fail before a journal is created.
 if s,e:=openProtectedJournalWithMaintenanceAt(journalDir,filepath.Join(root,"missing"));e==nil {
  if s!=nil{_ = s.Close()}
  t.Fatal("host store opened without maintenance directory")
 }
 if _,e=os.Stat(filepath.Join(journalDir,HostJournalFilename));!os.IsNotExist(e){
  t.Fatalf("unfenced opener created journal: %v",e)
 }
 s,e:=openProtectedJournalWithMaintenanceAt(journalDir,lockDir)
 if e!=nil{t.Fatal(e)}
 if s.maintenance==nil||!s.maintenance.held{t.Fatal("host Store lacks retained maintenance lease")}
 // Both Store sessions are allowed to coexist under shared maintenance.
 other,e:=openProtectedJournalWithMaintenanceAt(journalDir,lockDir)
 if e!=nil{t.Fatal(e)}
 if _,e=acquireMaintenanceLeaseAt(lockDir,true);!errors.Is(e,ErrMaintenanceBusy){
  t.Fatalf("exclusive maintenance bypassed live Store sessions: %v",e)
 }
 if e=other.Close();e!=nil{t.Fatal(e)}
 if _,e=acquireMaintenanceLeaseAt(lockDir,true);!errors.Is(e,ErrMaintenanceBusy){
  t.Fatalf("exclusive bypassed remaining Store: %v",e)
 }
 op:=operationFixture("hostlease-op","hostlease-project")
 if e=s.Begin(context.Background(),op);e!=nil{t.Fatal(e)}
 if e=s.Intent(context.Background(),op.ID,"PREPARE",fixedTime.Add(time.Second));e!=nil{t.Fatal(e)}
 if e=s.Close();e!=nil{t.Fatal(e)}
 exclusive,e:=acquireMaintenanceLeaseAt(lockDir,true)
 if e!=nil{t.Fatalf("closing last Store did not release shared lease: %v",e)}
 if opened,e:=openProtectedJournalWithMaintenanceAt(journalDir,lockDir);!errors.Is(e,ErrUnavailable){
  if opened!=nil{_ = opened.Close()}
  t.Fatalf("host Store bypassed exclusive maintenance holder: %v",e)
 }
 if e=exclusive.Release();e!=nil{t.Fatal(e)}
 reopened,e:=openProtectedJournalWithMaintenanceAt(journalDir,lockDir)
 if e!=nil{t.Fatal(e)}
 snap,e:=reopened.Inspect(context.Background(),op.ID)
 if e!=nil||!snap.BlocksNewWork{t.Fatalf("host session lost unresolved evidence: %#v %v",snap,e)}
 if e=reopened.Close();e!=nil{t.Fatal(e)}
 if e=os.Chmod(lockDir,0o770);e!=nil{t.Fatal(e)}
 if opened,e:=openProtectedJournalWithMaintenanceAt(journalDir,lockDir);!errors.Is(e,ErrUnavailable){
  if opened!=nil{_ = opened.Close()}
  t.Fatalf("untrusted maintenance directory accepted: %v",e)
 }
}

func TestFailedHostOpenReleasesSharedLeaseRootFixture(t *testing.T) {
 backupRootEnabled(t)
 root,e:=os.MkdirTemp("/root","docklantern-hostlease-failure-")
 if e!=nil{t.Fatal(e)}
 t.Cleanup(func(){_ = os.RemoveAll(root)})
 if e=os.Chmod(root,0o700);e!=nil{t.Fatal(e)}
 journalDir:=filepath.Join(root,"journal")
 lockDir:=filepath.Join(root,"locks")
 for _,dir:=range []string{journalDir,lockDir}{
  if e=os.Mkdir(dir,0o700);e!=nil{t.Fatal(e)}
 }
 s,e:=openProtectedJournalWithMaintenanceAt(journalDir,lockDir)
 if e!=nil{t.Fatal(e)}
 if _,e=s.db.Exec("PRAGMA user_version = 500");e!=nil{t.Fatal(e)}
 if e=s.Close();e!=nil{t.Fatal(e)}
 if opened,e:=openProtectedJournalWithMaintenanceAt(journalDir,lockDir);!errors.Is(e,ErrUnsupportedSchema){
  if opened!=nil{_ = opened.Close()}
  t.Fatalf("unsupported schema did not fail closed: %v",e)
 }
 lease,e:=acquireMaintenanceLeaseAt(lockDir,true)
 if e!=nil{t.Fatalf("failed host open leaked shared lease: %v",e)}
 if e=lease.Release();e!=nil{t.Fatal(e)}
}
