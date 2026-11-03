package executorjournal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func receiptRootFixture(t *testing.T, opID string) (*Store,*ProjectLock,string,string,string) {
	t.Helper()
	base,err:=os.MkdirTemp("/root","docklantern-receipts-")
	if err!=nil{t.Fatal(err)}
	t.Cleanup(func(){_ = os.RemoveAll(base)})
	if err:=os.Chmod(base,0o700);err!=nil{t.Fatal(err)}
	dir:=filepath.Join(base,"receipts")
	lockDir:=filepath.Join(base,"locks")
	if err:=os.Mkdir(dir,0o755);err!=nil{t.Fatal(err)}
	if err:=os.Chmod(dir,0o755);err!=nil{t.Fatal(err)}
	if err:=os.Mkdir(lockDir,0o700);err!=nil{t.Fatal(err)}
	lock,err:=acquireProjectLockAt(lockDir,"receipt-project")
	if err!=nil{t.Fatal(err)}
	t.Cleanup(func(){_ = lock.Release()})
	s,db:=journalFixture(t)
	ctx:=context.Background()
	op:=operationFixture(opID,"receipt-project")
	if err:=s.Begin(ctx,op);err!=nil{t.Fatal(err)}
	if err:=s.Intent(ctx,op.ID,"PREPARE",fixedTime.Add(time.Second));err!=nil{t.Fatal(err)}
	if err:=s.MarkUncertain(ctx,op.ID,"PREPARE","state_ambiguous",fixedTime.Add(2*time.Second));err!=nil{t.Fatal(err)}
	return s,lock,dir,db,filepath.Join(dir,opID+"-v1.json")
}

func TestReceiptOutboxRootFixture(t *testing.T) {
	if os.Geteuid()!=0 || os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1"{
		t.Skip("requires explicitly enabled disposable root-owned fixture")
	}
	s,lock,dir,_,target:=receiptRootFixture(t,"sanitized-demo")
	ctx:=context.Background()
	now:=fixedTime.Add(5*time.Second)
	if before,err:=s.ReceiptState(ctx,"sanitized-demo");err!=nil||before!="unknown"{
		t.Fatalf("absent receipt not unknown: %s %v",before,err)
	}
	result,err:=s.publishReceiptAt(ctx,lock,dir,"sanitized-demo",now,nil)
	if err!=nil||!result.FilePublished||result.AlreadyPublished||
		result.Outcome!=OutcomeNeedsReconciliation||!validHash(result.ContentSHA256) {
		t.Fatalf("safe receipt not published: %#v %v",result,err)
	}
	body,err:=os.ReadFile(target)
	if err!=nil{t.Fatal(err)}
	info,err:=os.Lstat(target)
	if err!=nil||!info.Mode().IsRegular()||info.Mode().Perm()!=0o644{
		t.Fatalf("published receipt must be root-owned 0644: %#v %v",info,err)
	}
	if len(body)>MaxReceiptBytes||receiptHash(body)!=result.ContentSHA256 {
		t.Fatal("receipt file bytes differ from durable journal hash")
	}
	if strings.Contains(string(body),"/root/")||strings.Contains(string(body),"password")||
		strings.Contains(string(body),"env_file")||strings.Contains(string(body),"docker_socket") {
		t.Fatal("non-allowlisted private data leaked")
	}
	receipt,err:=ParseReceipt(body)
	if err!=nil||receipt.Outcome!=OutcomeNeedsReconciliation||
		receipt.MessageCode!="verification_incomplete"||receipt.OperationID!="sanitized-demo" {
		t.Fatalf("unsafe public receipt: %#v %v",receipt,err)
	}
	repeated,err:=s.publishReceiptAt(ctx,lock,dir,"sanitized-demo",now.Add(time.Second),nil)
	if err!=nil||!repeated.AlreadyPublished||repeated.ContentSHA256!=result.ContentSHA256{
		t.Fatalf("repeat was not idempotent: %#v %v",repeated,err)
	}
	still,err:=os.ReadFile(target)
	if err!=nil||!bytes.Equal(body,still){t.Fatal("retry overwrote immutable receipt")}
	var pending,published int
	if err:=s.db.QueryRow(`SELECT count(*) FROM receipt_publications
		WHERE operation_id=? AND publication_state='pending'`,"sanitized-demo").Scan(&pending);err!=nil{t.Fatal(err)}
	if err:=s.db.QueryRow(`SELECT count(*) FROM receipt_publications
		WHERE operation_id=? AND publication_state='published'`,"sanitized-demo").Scan(&published);err!=nil{t.Fatal(err)}
	if pending!=1||published!=1{t.Fatalf("repeated publish appended events: %d/%d",pending,published)}
	view,err:=s.Inspect(ctx,"sanitized-demo")
	if err!=nil||view.Operation.Outcome!=OutcomeNeedsReconciliation||!view.BlocksNewWork{
		t.Fatalf("receipt publication unblocked operation: %#v %v",view,err)
	}
	if err:=s.Begin(ctx,operationFixture("new-op","receipt-project"));!errors.Is(err,ErrProjectBlocked){
		t.Fatalf("receipt publication authorized new operation: %v",err)
	}
}

func TestReceiptCrashRestartIdempotencyRootFixture(t *testing.T) {
	if os.Geteuid()!=0 || os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1"{
		t.Skip("requires explicitly enabled root-owned fixture")
	}
	for _,point:=range []string{"after_pending","after_file_sync","after_published"}{
		t.Run(point,func(t *testing.T){
			s,lock,dir,path,final:=receiptRootFixture(t,"crash-demo")
			ctx:=context.Background()
			_,err:=s.publishReceiptAt(ctx,lock,dir,"crash-demo",fixedTime.Add(5*time.Second),
				func(step string) error {
					if step==point{return errors.New("injected process crash")}
					return nil
				})
			if !errors.Is(err,ErrReceiptUnavailable){t.Fatalf("crash not propagated: %v",err)}
			before,err:=s.ReceiptState(ctx,"crash-demo")
			if err!=nil{t.Fatal(err)}
			if point=="after_published" && before!="published" ||
				point!="after_published" && before!="pending"{
				t.Fatalf("journal lost boundary intent at %s: %s",point,before)
			}
			if err:=s.Close();err!=nil{t.Fatal(err)}
			reopened,err:=openSQLite(path)
			if err!=nil{t.Fatal(err)}
			defer reopened.Close()
			current,err:=reopened.ReceiptState(ctx,"crash-demo")
			if err!=nil||current!=before{t.Fatalf("restart lost journal stage: %s %v",current,err)}
			firstBody,firstErr:=os.ReadFile(final)
			if point=="after_pending"{
				if !os.IsNotExist(firstErr){t.Fatalf("file published before crash point: %v",firstErr)}
			}else if firstErr!=nil{t.Fatal(firstErr)}
			result,err:=reopened.publishReceiptAt(ctx,lock,dir,"crash-demo",fixedTime.Add(6*time.Second),nil)
			if err!=nil||!result.FilePublished{
				t.Fatalf("retry did not reconcile durable receipt: %#v %v",result,err)
			}
			if result.AlreadyPublished!=(point=="after_published"){
				t.Fatalf("receipt prior-publication bit is wrong: %#v",result)
			}
			finalBody,err:=os.ReadFile(final)
			if err!=nil{t.Fatal(err)}
			if point!="after_pending"&&!bytes.Equal(firstBody,finalBody){
				t.Fatal("retry replaced pre-crash published bytes")
			}
			status,err:=reopened.ReceiptState(ctx,"crash-demo")
			if err!=nil||status!="published"{
				t.Fatalf("receipt was not durably reconciled: %s %v",status,err)
			}
			var count int
			if err:=reopened.db.QueryRow("SELECT count(*) FROM receipt_publications").Scan(&count);err!=nil||count!=2{
				t.Fatalf("replay added duplicate receipt events: %d %v",count,err)
			}
		})
	}
}

func TestReceiptOutboxRejectsUntrustedFilesystemRootFixture(t *testing.T) {
	if os.Geteuid()!=0 || os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1"{
		t.Skip("requires explicitly enabled root-only fixture")
	}
	for _,attack:=range []string{"symlink","fifo","wrong-mode","hardlink","nonroot-owner","group-writable-dir","symlink-ancestor","wrong-project-lock"}{
		t.Run(attack,func(t *testing.T){
			s,lock,dir,_,target:=receiptRootFixture(t,"attack-demo")
			ctx:=context.Background()
			if attack=="symlink"{
				if err:=os.Symlink("/etc/passwd",target);err!=nil{t.Fatal(err)}
			}
			if attack=="fifo"{
				if err:=syscall.Mkfifo(target,0o600);err!=nil{t.Fatal(err)}
			}
			if attack=="wrong-mode"||attack=="hardlink"||attack=="nonroot-owner"{
				r,err:=s.receiptForOperation(ctx,"attack-demo")
				if err!=nil{t.Fatal(err)}
				b,err:=r.canonical()
				if err!=nil{t.Fatal(err)}
				if err:=os.WriteFile(target,b,0o644);err!=nil{t.Fatal(err)}
				if err:=os.Chmod(target,0o644);err!=nil{t.Fatal(err)}
				if attack=="wrong-mode"{
					if err:=os.Chmod(target,0o666);err!=nil{t.Fatal(err)}
				}
				if attack=="hardlink"{
					if err:=os.Link(target,target+".linked");err!=nil{t.Fatal(err)}
				}
				if attack=="nonroot-owner"{
					if err:=os.Chown(target,65534,-1);err!=nil{t.Fatal(err)}
				}
			}
			if attack=="group-writable-dir"{
				if err:=os.Chmod(dir,0o777);err!=nil{t.Fatal(err)}
			}
			if attack=="symlink-ancestor"{
				realDir:=dir+"-real"
				if err:=os.Rename(dir,realDir);err!=nil{t.Fatal(err)}
				if err:=os.Symlink(realDir,dir);err!=nil{t.Fatal(err)}
			}
			if attack=="wrong-project-lock"{
				lock.projectID="different-project"
			}
			if _,err:=s.publishReceiptAt(ctx,lock,dir,"attack-demo",fixedTime.Add(5*time.Second),nil);
				!errors.Is(err,ErrReceiptUnavailable) {
				t.Fatalf("unsafe outbox accepted attack %q: %v",attack,err)
			}
			if attack=="symlink"{
				info,err:=os.Lstat(target)
				if err!=nil||info.Mode()&os.ModeSymlink==0{
					t.Fatal("unsafe outbox modified existing symlink")
				}
			}
		})
	}
}

func TestReceiptOutboxQuotaAndUnknownNamesRootFixture(t *testing.T) {
	if os.Geteuid()!=0 || os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1"{
		t.Skip("requires explicitly enabled root-only fixture")
	}
	s,lock,dir,_,target:=receiptRootFixture(t,"capacity-demo")
	ctx:=context.Background()
	for i:=0;i<MaxPublishedReceipts;i++{
		name:=fmt.Sprintf("history-%04d-v1.json",i)
		if err:=os.WriteFile(filepath.Join(dir,name),[]byte("historical"),0o644);err!=nil{t.Fatal(err)}
	}
	if _,err:=s.publishReceiptAt(ctx,lock,dir,"capacity-demo",fixedTime.Add(5*time.Second),nil);
		!errors.Is(err,ErrReceiptUnavailable) {
		t.Fatal("1001st receipt was accepted or rotated away unresolved history")
	}
	if _,err:=os.Lstat(target);!os.IsNotExist(err){
		t.Fatal("receipt written despite capacity guard")
	}
	state,err:=s.ReceiptState(ctx,"capacity-demo")
	if err!=nil||state!="pending"{
		t.Fatalf("quota denial lost durable pending intent: %q %v",state,err)
	}
	if err:=os.Remove(filepath.Join(dir,"history-0000-v1.json"));err!=nil{t.Fatal(err)}
	if _,err:=s.publishReceiptAt(ctx,lock,dir,"capacity-demo",fixedTime.Add(6*time.Second),nil);err!=nil{
		t.Fatalf("capacity recovery failed: %v",err)
	}
	// Unknown names cannot trigger deletion or rotation in this release.
	if err:=os.WriteFile(filepath.Join(dir,"unexpected.secret"),[]byte("x"),0o600);err!=nil{t.Fatal(err)}
	// A known pending operation may never use a broad/wildcard cleanup.
	entries,err:=os.ReadDir(dir)
	if err!=nil||len(entries)!=MaxPublishedReceipts+1{
		t.Fatalf("unexpected quota recovery/unknown-file count: %d %v",len(entries),err)
	}
}

func TestReceiptFailsClosedIfJournalChangesBetweenPendingAndPublishedRootFixture(t *testing.T) {
	if os.Geteuid()!=0 || os.Getenv("DOCKLANTERN_JOURNAL_ROOT_FIXTURE")!="1"{
		t.Skip("requires root-owned fixture")
	}
	s,lock,dir,_,target:=receiptRootFixture(t,"changed-demo")
	_,err:=s.publishReceiptAt(context.Background(),lock,dir,"changed-demo",
		fixedTime.Add(5*time.Second),func(stage string)error{
			if stage=="after_pending" {
				_,e:=s.db.Exec("UPDATE operations SET updated_ns=updated_ns+1 WHERE operation_id='changed-demo'")
				return e
			}
			return nil
		})
	if !errors.Is(err,ErrReceiptUnavailable){
		t.Fatalf("changed journal mistakenly published: %v",err)
	}
	if _,err:=os.Lstat(target);!os.IsNotExist(err){
		t.Fatal("receipt file existed despite changed journal")
	}
	if state,err:=s.ReceiptState(context.Background(),"changed-demo");err!=nil||state!="pending"{
		t.Fatalf("changed journal unexpectedly recorded published: %s %v",state,err)
	}
}
