package executorjournal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This fixture operates only under a disposable /root directory on the
// isolated CI runner. It does NOT create production /var/lib journal files.
func TestProtectedJournalRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1" || os.Geteuid()!=0 {
		t.Skip("requires explicitly enabled disposable root-owned fixture")
	}
	base,err:=os.MkdirTemp("/root","docklantern-journal-")
	if err!=nil {t.Fatal(err)}
	t.Cleanup(func(){_ = os.RemoveAll(base)})
	if err:=os.Chmod(base,0o700);err!=nil {t.Fatal(err)}
	dir:=filepath.Join(base,"journal")
	if err:=os.Mkdir(dir,0o700);err!=nil {t.Fatal(err)}
	dbfile:=filepath.Join(dir,HostJournalFilename)

	s,err:=openProtectedJournal(dir)
	if err!=nil {t.Fatalf("fresh root-protected SQLite journal rejected: %v",err)}
	op:=operationFixture("root-fixture-op","root-fixture-project")
	if err:=s.Begin(context.Background(),op);err!=nil {t.Fatal(err)}
	if err:=s.Intent(context.Background(),op.ID,"PREPARE",fixedTime.Add(time.Second));err!=nil {t.Fatal(err)}
	if err:=s.Close();err!=nil {t.Fatal(err)}
	info,err:=os.Lstat(dbfile)
	if err!=nil||!info.Mode().IsRegular()||info.Mode().Perm()!=0o600 {
		t.Fatalf("private SQLite journal owner/mode mismatch: %#v %v",info,err)
	}
	for _,suffix:=range []string{"-wal","-shm"} {
		path:=dbfile+suffix
		if inf,err:=os.Lstat(path);err==nil && inf.Mode().Perm()!=0o600 {
			t.Fatalf("%s sidecar permissions unsafe: %v",suffix,inf.Mode())
		}
	}
	s,err=openProtectedJournal(dir)
	if err!=nil {t.Fatalf("reopen root protected journal rejected: %v",err)}
	recovered,err:=s.Inspect(context.Background(),op.ID)
	if err!=nil||recovered.EffectiveOutcome!=OutcomeNeedsReconciliation {
		t.Fatalf("root-owned crash intent vanished: %#v %v",recovered,err)
	}
	s.Close()

	reject:=func(label string){
		t.Helper()
		if opened,err:=openProtectedJournal(dir);!errors.Is(err,ErrUnavailable) {
			if opened!=nil {opened.Close()}
			t.Fatalf("unsafe root journal opened (%s): %v",label,err)
		}
	}
	if err:=os.Chmod(dbfile,0o640);err!=nil {t.Fatal(err)}
	reject("group-readable SQLite file")
	if err:=os.Chmod(dbfile,0o600);err!=nil {t.Fatal(err)}
	hard:=filepath.Join(dir,"outside-name")
	if err:=os.Link(dbfile,hard);err!=nil {t.Fatal(err)}
	reject("hardlinked SQLite file")
	if err:=os.Remove(hard);err!=nil {t.Fatal(err)}
	if err:=os.Chmod(dir,0o770);err!=nil {t.Fatal(err)}
	reject("group writable journal directory")
	if err:=os.Chmod(dir,0o700);err!=nil {t.Fatal(err)}
	if err:=os.Chmod(base,0o770);err!=nil {t.Fatal(err)}
	reject("group writable ancestor")
	if err:=os.Chmod(base,0o700);err!=nil {t.Fatal(err)}

	if err:=os.Rename(dbfile,dbfile+".original");err!=nil {t.Fatal(err)}
	if err:=os.Symlink("/etc/passwd",dbfile);err!=nil {t.Fatal(err)}
	reject("symlinked SQLite database")
	if err:=os.Remove(dbfile);err!=nil {t.Fatal(err)}
	if err:=os.Rename(dbfile+".original",dbfile);err!=nil {t.Fatal(err)}

	// A nonroot monitor-owned file may never become executor journal authority.
	if err:=os.Chown(dbfile,65534,-1);err!=nil {t.Fatal(err)}
	reject("nonroot-owned SQLite file")
	if err:=os.Chown(dbfile,0,-1);err!=nil {t.Fatal(err)}

	realDir:=dir+"-safe"
	if err:=os.Rename(dir,realDir);err!=nil {t.Fatal(err)}
	if err:=os.Symlink(realDir,dir);err!=nil {t.Fatal(err)}
	reject("symlinked executor journal directory")
	if err:=os.Remove(dir);err!=nil {t.Fatal(err)}
	if err:=os.Rename(realDir,dir);err!=nil {t.Fatal(err)}
	s,err=openProtectedJournal(dir)
	if err!=nil {t.Fatalf("fixture not restored: %v",err)}
	s.Close()

	// Roots and relative or unclean paths may never be used as journal stores.
	for _,path:=range []string{
		"", "/", "journal", "/root/../var/lib", "/root//journal",
		"/root/"+strings.Repeat("x",600),
	} {
		if opened,err:=openProtectedJournal(path);err==nil {
			opened.Close()
			t.Fatalf("unsafe host journal path accepted: %q",path)
		}
	}
}

func TestProtectedJournalCorruptionAndFutureSchemaRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1" || os.Geteuid()!=0 {
		t.Skip("requires explicitly enabled root-only fixture")
	}
	base,err:=os.MkdirTemp("/root","docklantern-journal-future-")
	if err!=nil {t.Fatal(err)}
	t.Cleanup(func(){_ = os.RemoveAll(base)})
	if err:=os.Chmod(base,0o700);err!=nil {t.Fatal(err)}
	dir:=filepath.Join(base,"journal")
	if err:=os.Mkdir(dir,0o700);err!=nil {t.Fatal(err)}
	s,err:=openProtectedJournal(dir)
	if err!=nil {t.Fatal(err)}
	if _,err:=s.db.Exec("PRAGMA user_version = 500");err!=nil {t.Fatal(err)}
	if err:=s.Close();err!=nil {t.Fatal(err)}
	if _,err:=openProtectedJournal(dir);!errors.Is(err,ErrUnsupportedSchema) {
		t.Fatalf("unknown future journal version opened as writable: %v",err)
	}
	// This fixture never attempts to coerce/migrate future versions or
	// overwrite existing operations to get a passing test.
	_ = syscall.O_NOFOLLOW
}
