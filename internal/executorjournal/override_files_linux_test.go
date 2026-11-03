package executorjournal

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func overrideStageRootFixture(t *testing.T) (*Store,*ProjectLock,string,string,OverrideManifest,string) {
	t.Helper()
	root,err:=os.MkdirTemp("/root","docklantern-override-stage-")
	if err!=nil {t.Fatal(err)}
	t.Cleanup(func(){_ = os.RemoveAll(root)})
	if err:=os.Chmod(root,0o700);err!=nil {t.Fatal(err)}
	locks:=filepath.Join(root,"locks")
	overrides:=filepath.Join(root,"overrides")
	project:="override-demo"
	for _,name:=range []string{locks,overrides,filepath.Join(overrides,project)} {
		if err:=os.Mkdir(name,0o700);err!=nil {t.Fatal(err)}
	}
	lock,err:=acquireProjectLockAt(locks,project)
	if err!=nil {t.Fatal(err)}
	t.Cleanup(func(){_ = lock.Release()})
	s,path:=journalFixture(t)
	op:=operationFixture("override-operation",project)
	if err:=s.Begin(context.Background(),op);err!=nil {t.Fatal(err)}
	basehash:="sha256:"+strings.Repeat("a",64)
	_,err=s.db.Exec(`INSERT INTO override_records(project_id,generation,committed_sha256)
		VALUES(?,0,?)`,project,basehash)
	if err!=nil {t.Fatal(err)}
	for i,phase:=range []string{"PREPARE","AUTHORIZE","PREFLIGHT","PULL"} {
		at:=fixedTime.Add(time.Duration(2*i+1)*time.Second)
		if err:=s.Intent(context.Background(),op.ID,phase,at);err!=nil {t.Fatal(err)}
		if err:=s.Complete(context.Background(),op.ID,phase,at.Add(time.Second));err!=nil {t.Fatal(err)}
	}
	if err:=s.Intent(context.Background(),op.ID,"STAGE",fixedTime.Add(9*time.Second));err!=nil {t.Fatal(err)}
	m:=OverrideManifest{
		SchemaVersion:1,ProjectID:project,OperationID:op.ID,
		Generation:1,PreviousGeneration:0,
		PreviousCommittedSHA256:basehash,CandidateImageDigest:op.CandidateDigest,
	}
	return s,lock,overrides,path,m,filepath.Join(overrides,project,overrideFilename(1))
}

func TestOverrideGenerationStageAndPromotionIntentRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1" || os.Geteuid()!=0 {
		t.Skip("requires isolated root-owned disposable fixture")
	}
	s,lock,root,_,m,file:=overrideStageRootFixture(t)
	ctx:=context.Background()
	hash,err:=s.stageOverrideAt(ctx,lock,root,m,fixedTime.Add(10*time.Second),nil)
	if err!=nil||!validHash(hash){t.Fatalf("protected metadata stage rejected: %v",err)}
	info,err:=os.Lstat(file)
	if err!=nil||!info.Mode().IsRegular()||info.Mode().Perm()!=0o600 {
		t.Fatalf("staged manifest not a private 0600 regular file: %#v %v",info,err)
	}
	want,err:=m.bytes()
	if err!=nil {t.Fatal(err)}
	actual,err:=os.ReadFile(file)
	if err!=nil||!bytes.Equal(actual,want) {t.Fatalf("staged metadata mismatch: %v",err)}
	if bytes.Contains(actual,[]byte("compose.yaml"))||
		bytes.Contains(actual,[]byte("/etc/"))||bytes.Contains(actual,[]byte("env_file")) {
		t.Fatal("metadata manifest contains paths or Compose configuration")
	}
	first,err:=s.InspectOverrideGeneration(ctx,m.ProjectID)
	if err!=nil || first.CurrentStage!="staged" || first.Committed || !first.NeedsReconciliation ||
		first.ManifestSHA256!=hash {
		t.Fatalf("metadata staging incorrectly treated as committed: %#v %v",first,err)
	}
	if _,err:=s.stageOverrideAt(ctx,lock,root,m,fixedTime.Add(10*time.Second),nil);!errors.Is(err,ErrOverrideStage) {
		t.Fatalf("duplicate generation overwritten: %v",err)
	}
	still,err:=os.ReadFile(file)
	if err!=nil||!bytes.Equal(still,want){t.Fatal("duplicate stage mutated existing generation")}
	if err:=s.Complete(ctx,m.OperationID,"STAGE",fixedTime.Add(11*time.Second));err!=nil {t.Fatal(err)}
	for _,entry:=range []struct{phase string;intent,complete int}{
		{"APPLY",12,13},{"VERIFY",14,15},
	} {
		if err:=s.Intent(ctx,m.OperationID,entry.phase,fixedTime.Add(time.Duration(entry.intent)*time.Second));err!=nil {t.Fatal(err)}
		if err:=s.Complete(ctx,m.OperationID,entry.phase,fixedTime.Add(time.Duration(entry.complete)*time.Second));err!=nil {t.Fatal(err)}
	}
	if err:=s.Intent(ctx,m.OperationID,"COMMIT",fixedTime.Add(16*time.Second));err!=nil {t.Fatal(err)}
	if err:=s.promotionIntentAt(ctx,lock,root,m,"sha256:"+strings.Repeat("0",64),fixedTime.Add(17*time.Second));!errors.Is(err,ErrOverrideStage) {
		t.Fatal("promotion intent accepted mismatched manifest hash")
	}
	if err:=s.promotionIntentAt(ctx,lock,root,m,hash,fixedTime.Add(17*time.Second));err!=nil {
		t.Fatalf("promotion intent metadata rejected: %v",err)
	}
	if err:=s.promotionIntentAt(ctx,lock,root,m,hash,fixedTime.Add(18*time.Second));!errors.Is(err,ErrOverrideStage) {
		t.Fatalf("duplicate promotion intent appended: %v",err)
	}
	view,err:=s.InspectOverrideGeneration(ctx,m.ProjectID)
	if err!=nil||view.CurrentStage!="promotion_intent"||view.Committed||!view.NeedsReconciliation {
		t.Fatalf("promotion intent falsely treated as committed: %#v %v",view,err)
	}
	var committed int64
	if err:=s.db.QueryRow("SELECT max(generation) FROM override_records WHERE project_id=?",m.ProjectID).Scan(&committed);err!=nil||committed!=0 {
		t.Fatalf("promotion intent changed committed generation: %d %v",committed,err)
	}
	if err:=s.Complete(ctx,m.OperationID,"COMMIT",fixedTime.Add(18*time.Second));!errors.Is(err,ErrInvalidTransition) {
		t.Fatalf("unfinished promotion incorrectly allowed COMMIT completion: %v",err)
	}
}

func TestOverrideGenerationCrashBoundariesRemainIncompleteRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1" || os.Geteuid()!=0 {
		t.Skip("requires isolated root-owned disposable fixture")
	}
	for _,point:=range []string{"after_stage_intent","after_manifest_sync","after_staged_event"} {
		t.Run(point,func(t *testing.T){
			s,lock,root,db,m,file:=overrideStageRootFixture(t)
			_,err:=s.stageOverrideAt(context.Background(),lock,root,m,
				fixedTime.Add(10*time.Second),func(stage string)error{
					if stage==point{return errors.New("simulated abrupt interruption")}
					return nil
				})
			if !errors.Is(err,ErrOverrideStage){t.Fatalf("crash not propagated: %v",err)}
			if err:=s.Close();err!=nil {t.Fatal(err)}
			reopened,err:=openSQLite(db)
			if err!=nil {t.Fatal(err)}
			defer reopened.Close()
			view,err:=reopened.InspectOverrideGeneration(context.Background(),m.ProjectID)
			if err!=nil||!view.NeedsReconciliation||view.Committed {
				t.Fatalf("interrupted stage silently committed: %#v %v",view,err)
			}
			if point=="after_staged_event" && view.CurrentStage!="staged" {
				t.Fatalf("lost staged event after crash: %#v",view)
			}
			if point!="after_staged_event" && view.CurrentStage!="stage_intent" {
				t.Fatalf("lost durable intent after crash: %#v",view)
			}
			_,fileErr:=os.Lstat(file)
			if point=="after_stage_intent" {
				if !os.IsNotExist(fileErr){t.Fatalf("file written before crash boundary: %v",fileErr)}
			}else if fileErr!=nil {
				t.Fatalf("crash lost already synced private manifest: %v",fileErr)
			}
			if err:=reopened.Begin(context.Background(),operationFixture("new-override-op",m.ProjectID));!errors.Is(err,ErrProjectBlocked){
				t.Fatalf("interrupted stage allowed another operation: %v",err)
			}
			if err:=reopened.appendStaged(context.Background(),m,view.ManifestSHA256,
				fixedTime.Add(time.Hour));!errors.Is(err,ErrOverrideStage) {
				t.Fatalf("restarted journal replayed staging completion: %v",err)
			}
		})
	}
}

func TestOverrideGenerationRejectsUnsafeFilesAndLocksRootFixture(t *testing.T) {
	if os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1" || os.Geteuid()!=0 {
		t.Skip("requires isolated root-owned disposable fixture")
	}
	t.Run("unsafe-override-directory",func(t *testing.T){
		s,lock,root,_,m,_:=overrideStageRootFixture(t)
		if err:=os.Chmod(root,0o770);err!=nil {t.Fatal(err)}
		if _,err:=s.stageOverrideAt(context.Background(),lock,root,m,
			fixedTime.Add(10*time.Second),nil);!errors.Is(err,ErrOverrideStage) {
			t.Fatal("group-writable override directory was accepted")
		}
		var n int
		if err:=s.db.QueryRow("SELECT count(*) FROM override_generation_events").Scan(&n);err!=nil||n!=0{
			t.Fatalf("invalid parent generated stage intent: %d %v",n,err)
		}
	})
	t.Run("preexisting-symlink",func(t *testing.T){
		s,lock,root,_,m,file:=overrideStageRootFixture(t)
		if err:=os.Symlink("/etc/passwd",file);err!=nil {t.Fatal(err)}
		if _,err:=s.stageOverrideAt(context.Background(),lock,root,m,
			fixedTime.Add(10*time.Second),nil);!errors.Is(err,ErrOverrideStage) {
			t.Fatal("existing symlink was overwritten")
		}
		info,err:=os.Lstat(file)
		if err!=nil||info.Mode()&os.ModeSymlink==0 {t.Fatalf("symlink modified: %v",err)}
	})
	t.Run("hardlink-detected-after-stage",func(t *testing.T){
		s,lock,root,_,m,file:=overrideStageRootFixture(t)
		if _,err:=s.stageOverrideAt(context.Background(),lock,root,m,
			fixedTime.Add(10*time.Second),nil);err!=nil{t.Fatal(err)}
		hard:=file+".linked"
		if err:=os.Link(file,hard);err!=nil {t.Fatal(err)}
		view,err:=s.InspectOverrideGeneration(context.Background(),m.ProjectID)
		if err!=nil {t.Fatal(err)}
		dirfd,err:=openOverrideProjectDir(root,m.ProjectID)
		if err!=nil {t.Fatal(err)}
		defer syscall.Close(dirfd)
		if err:=verifyManifest(dirfd,m.Generation,view.ManifestSHA256);!errors.Is(err,ErrOverrideStage) {
			t.Fatal("hardlinked manifest accepted")
		}
	})
	t.Run("wrong-lock-or-operation",func(t *testing.T){
		s,lock,root,_,m,_:=overrideStageRootFixture(t)
		m.ProjectID="other-project"
		if _,err:=s.stageOverrideAt(context.Background(),lock,root,m,
			fixedTime.Add(10*time.Second),nil);!errors.Is(err,ErrOverrideStage){t.Fatal("wrong project lock accepted")}
		m.ProjectID="override-demo"
		if err:=lock.Release();err!=nil {t.Fatal(err)}
		if _,err:=s.stageOverrideAt(context.Background(),lock,root,m,
			fixedTime.Add(10*time.Second),nil);!errors.Is(err,ErrOverrideStage){t.Fatal("released project lock accepted")}
	})
}
